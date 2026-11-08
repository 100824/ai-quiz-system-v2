package v2

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type DesignImageService struct {
	repo         *Repository
	dir          string
	url          string
	client       *http.Client
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	workerLock   *os.File
	htmlDir      string
	htmlHandler  *Handler
	claimMu      sync.Mutex
	imageBuffers chan struct{}
}

func (h *Handler) StartDesignImages(ctx context.Context, dir string) error {
	s, err := newDesignImageService(h.repo, dir, "https://ark.cn-beijing.volces.com/api/v3/images/generations", http.DefaultClient)
	if err != nil {
		return err
	}
	s.htmlDir = filepath.Join(filepath.Dir(s.dir), "design-html")
	s.htmlHandler = h
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
	if client == nil {
		client = http.DefaultClient
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &DesignImageService{repo: repo, dir: dir, htmlDir: filepath.Join(dir, "html"), url: url, client: client, imageBuffers: make(chan struct{}, 2)}, nil
}
func (s *DesignImageService) Start(parent context.Context) error {
	if s.workerLock != nil {
		return fmt.Errorf("作品生成任务服务已启动")
	}
	if err := os.MkdirAll(s.htmlDir, 0700); err != nil {
		return err
	}
	// Lock the database owner before recovering jobs. A second process must not
	// mark another process's in-flight, potentially paid requests as interrupted.
	var databaseFile string
	if err := s.repo.db.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&databaseFile); err != nil {
		return err
	}
	lockPath := filepath.Join(s.dir, ".worker.lock")
	if databaseFile != "" {
		lockPath = databaseFile + ".design-images.lock"
	}
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return fmt.Errorf("同一数据库已有作品生成进程，拒绝重复启动：%w", err)
	}
	if err := s.repo.RecoverDesignImageJobs(); err != nil {
		lock.Close()
		return err
	}
	if err := s.repo.RecoverDesignHTMLJobs(); err != nil {
		lock.Close()
		return err
	}
	s.workerLock = lock
	log.Printf("design generation worker started: pid=%d", os.Getpid())
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	for n := 0; n < 10; n++ {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			for {
				if ctx.Err() != nil {
					return
				}
				j, hj, err := s.claimNextGeneration()
				if err == nil {
					if hj != nil {
						s.runHTML(ctx, hj)
					} else {
						s.run(ctx, j)
					}
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
		s.cancel = nil
	}
	if s.workerLock != nil {
		s.workerLock.Close()
		s.workerLock = nil
	}
}

func (s *DesignImageService) imagePath(file string) (string, error) {
	if file == "" || filepath.Base(file) != file {
		return "", fmt.Errorf("图片路径无效")
	}
	return filepath.Join(s.dir, file), nil
}

func (s *DesignImageService) run(parent context.Context, j *DesignImageJob) {
	defer func() {
		if failure := recover(); failure != nil {
			log.Printf("design image job %s panic (%T)", j.ID, failure)
			_ = s.repo.FailDesignImageJob(j, "图片生成出现异常，请稍后重试。")
		}
	}()
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
		log.Printf("design image job %s failed: %v", j.ID, err)
		text := "图片生成失败，请重试。"
		var bounded designArtifactError
		if errors.As(err, &bounded) {
			text = bounded.Error()
		}
		if ctx.Err() != nil {
			text = "图片生成超时或中断，请重试。"
		}
		if e := s.repo.FailDesignImageJob(j, text); e != nil {
			log.Printf("design image job state update failed: %s: %v", j.ID, e)
		}
	}
}

func (s *DesignImageService) generate(ctx context.Context, j *DesignImageJob) (*DesignImage, error) {
	if err := checkDesignStorage(s.dir, designImageLimit); err != nil {
		return nil, err
	}
	key := envFirst("ARK_API_KEY", "ARK_KEY")
	if key == "" {
		return nil, fmt.Errorf("未配置图片模型密钥")
	}
	payload, err := s.imageRequestBody(ctx, j)
	if err != nil {
		return nil, err
	}
	defer payload.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	req.ContentLength = payload.size
	response, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("图片服务网络请求失败")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("图片服务状态异常：%d", response.StatusCode)
	}
	// Network generation still has ten slots. Only large response decoding and
	// saving share two memory slots, avoiding ten simultaneous image allocations.
	select {
	case s.imageBuffers <- struct{}{}:
		defer func() { <-s.imageBuffers }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	data, config, format, err := decodeDesignImageResponse(response.Body)
	if err != nil {
		return nil, err
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
