package v2

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type DesignImageService struct {
	repo   *Repository
	dir    string
	url    string
	client *http.Client
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func (h *Handler) StartDesignImages(ctx context.Context, dir string) error {
	s, err := newDesignImageService(h.repo, dir, "https://ark.cn-beijing.volces.com/api/v3/images/generations", http.DefaultClient)
	if err != nil {
		return err
	}
	if err = s.Start(ctx); err != nil {
		return err
	}
	h.designImages = s
	return nil
}
func (h *Handler) StopDesignImages() {
	if h.designImages != nil {
		h.designImages.Close()
	}
}
func newDesignImageService(repo *Repository, dir, url string, client *http.Client) (*DesignImageService, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &DesignImageService{repo: repo, dir: dir, url: url, client: client}, nil
}
func (s *DesignImageService) Start(parent context.Context) error {
	if err := s.repo.RecoverDesignImageJobs(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	for n := 0; n < 2; n++ {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			for {
				if ctx.Err() != nil {
					return
				}
				j, err := s.repo.ClaimDesignImageJob()
				if err == nil {
					s.run(ctx, j)
					continue
				}
				if err != sql.ErrNoRows {
					log.Printf("design image queue: %v", err)
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}
			}
		}()
	}
	return nil
}
func (s *DesignImageService) Close() {
	if s.cancel != nil {
		s.cancel()
		s.wg.Wait()
	}
}

func (s *DesignImageService) imagePath(file string) (string, error) {
	if file == "" || filepath.Base(file) != file {
		return "", fmt.Errorf("图片路径无效")
	}
	return filepath.Join(s.dir, file), nil
}

func (s *DesignImageService) run(parent context.Context, j *DesignImageJob) {
	ctx, cancel := context.WithTimeout(parent, 240*time.Second)
	defer cancel()
	i, err := s.generate(ctx, j)
	if err == nil {
		err = s.repo.CompleteDesignImageJob(j, i)
		if err != nil {
			if p, e := s.imagePath(i.FileName); e == nil {
				_ = os.Remove(p)
			}
		}
	}
	if err != nil {
		// All upstream failures are converted to controlled messages; never log keys, raw bodies or prompts.
		log.Printf("design image job %s failed", j.ID)
		text := "图片生成失败，请重试。"
		if ctx.Err() != nil {
			text = "图片生成超时或中断，请重试。"
		}
		if e := s.repo.FailDesignImageJob(j, text); e != nil {
			log.Printf("design image job state update failed: %s", j.ID)
		}
	}
}

func (s *DesignImageService) generate(ctx context.Context, j *DesignImageJob) (*DesignImage, error) {
	key := envFirst("ARK_API_KEY", "ARK_KEY")
	if key == "" {
		return nil, fmt.Errorf("未配置图片模型密钥")
	}
	body := map[string]any{"model": designImageModel, "prompt": j.Prompt, "size": "1K", "response_format": "b64_json", "output_format": "jpeg"}
	if j.SourceImageID != "" {
		previous, err := scanDesignImage(s.repo.db.QueryRow(`SELECT `+designImageColumns+` FROM design_images WHERE id=? AND session_id=?`, j.SourceImageID, j.SessionID))
		if err != nil {
			return nil, err
		}
		path, err := s.imagePath(previous.FileName)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		body["image"] = "data:" + http.DetectContentType(data) + ";base64," + base64.StdEncoding.EncodeToString(data)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	response, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("图片服务网络请求失败")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("图片服务状态异常：%d", response.StatusCode)
	}
	var result struct {
		Error json.RawMessage `json:"error"`
		Data  []struct {
			Base64 string `json:"b64_json"`
		} `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 64<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("图片服务响应格式错误")
	}
	if len(result.Error) > 0 && string(result.Error) != "null" || len(result.Data) != 1 {
		return nil, fmt.Errorf("图片服务未返回单张图片")
	}
	data, err := base64.StdEncoding.DecodeString(result.Data[0].Base64)
	if err != nil || len(data) == 0 || len(data) > 32<<20 {
		return nil, fmt.Errorf("图片数据无效")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 32_000_000 {
		return nil, fmt.Errorf("图片格式或尺寸无效")
	}
	i := &DesignImage{ID: designRandomID(), Width: config.Width, Height: config.Height}
	ext := ".jpg"
	if format == "png" {
		ext = ".png"
	}
	i.FileName = i.ID + ext
	tmp, err := os.CreateTemp(s.dir, ".pending-*"+ext)
	if err != nil {
		return nil, err
	}
	tempName := tmp.Name()
	defer os.Remove(tempName)
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return nil, err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return nil, err
	}
	if err = tmp.Close(); err != nil {
		return nil, err
	}
	path, _ := s.imagePath(i.FileName)
	if err = os.Rename(tempName, path); err != nil {
		return nil, err
	}
	return i, nil
}
