package v2

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"os"
)

const designImageLimit = 4 << 20
const designImageReferenceLimit = 8 << 20 // Retain compatibility with earlier PNG works.
const designImageResponseLimit = ((designImageLimit+2)/3)*4 + (16 << 10)

type designArtifactError string

func (e designArtifactError) Error() string { return string(e) }

type designImagePayload struct {
	io.ReadCloser
	size int64
	ctx  context.Context
}

func (p *designImagePayload) Read(data []byte) (int, error) {
	if err := p.ctx.Err(); err != nil {
		return 0, err
	}
	return p.ReadCloser.Read(data)
}

// The reference is streamed from disk through the base64 encoder, so ten image
// edits do not hold ten complete files and multiple encoded copies in memory.
func (s *DesignImageService) imageRequestBody(ctx context.Context, j *DesignImageJob) (*designImagePayload, error) {
	if _, err := normalizeDesignText(j.Prompt, "生成要求", designPromptLimit, true); err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{"model": designImageModel, "prompt": j.Prompt, "size": "1K", "response_format": "b64_json", "output_format": "jpeg"})
	if err != nil {
		return nil, err
	}
	if j.SourceImageID == "" {
		return &designImagePayload{io.NopCloser(bytes.NewReader(body)), int64(len(body)), ctx}, nil
	}
	previous, err := scanDesignImage(s.repo.db.QueryRow(`SELECT `+designImageColumns+` FROM design_images WHERE id=? AND session_id=?`, j.SourceImageID, j.SessionID))
	if err != nil {
		return nil, err
	}
	path, err := s.imagePath(previous.FileName)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() > designImageReferenceLimit {
		f.Close()
		return nil, designArtifactError("原图片无法读取或超过8MiB，请联系老师")
	}
	config, format, err := image.DecodeConfig(io.LimitReader(f, designImageReferenceLimit))
	if err != nil || (format != "jpeg" && format != "png") || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 32_000_000 {
		f.Close()
		return nil, designArtifactError("原图片格式或尺寸无效，请联系老师")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, err
	}
	reader, writer := io.Pipe()
	prefix := append(body[:len(body)-1], []byte(fmt.Sprintf(`,"image":"data:image/%s;base64,`, format))...)
	stop := context.AfterFunc(ctx, func() { _ = reader.CloseWithError(ctx.Err()) })
	go func() {
		defer f.Close()
		defer stop()
		_, err := writer.Write(prefix)
		encoder := base64.NewEncoder(base64.StdEncoding, writer)
		if err == nil {
			var n int64
			n, err = io.Copy(encoder, io.LimitReader(f, designImageReferenceLimit+1))
			if n > designImageReferenceLimit {
				err = fmt.Errorf("reference image exceeds limit")
			}
		}
		if err == nil {
			err = encoder.Close()
		}
		if err == nil {
			_, err = io.WriteString(writer, `"}`)
		}
		_ = writer.CloseWithError(err)
	}()
	return &designImagePayload{reader, int64(len(prefix) + base64.StdEncoding.EncodedLen(int(stat.Size())) + 2), ctx}, nil
}

func decodeDesignImageResponse(body io.Reader) ([]byte, image.Config, string, error) {
	limited := &io.LimitedReader{R: body, N: designImageResponseLimit + 1}
	decoder := json.NewDecoder(limited)
	var result struct {
		Error json.RawMessage `json:"error"`
		Data  []struct {
			Base64 string `json:"b64_json"`
		} `json:"data"`
	}
	err := decoder.Decode(&result)
	if limited.N <= 0 {
		return nil, image.Config{}, "", designArtifactError("图片响应过大，请简化要求后重试")
	}
	if err != nil {
		return nil, image.Config{}, "", designArtifactError("图片响应不完整，请重试")
	}
	var extra any
	err = decoder.Decode(&extra)
	if limited.N <= 0 {
		return nil, image.Config{}, "", designArtifactError("图片响应过大，请简化要求后重试")
	}
	if err != io.EOF || len(result.Error) > 0 && string(result.Error) != "null" || len(result.Data) != 1 {
		return nil, image.Config{}, "", designArtifactError("图片服务未返回有效的单张图片，请重试")
	}
	encoded := result.Data[0].Base64
	if len(encoded) == 0 || len(encoded) > base64.StdEncoding.EncodedLen(designImageLimit) {
		return nil, image.Config{}, "", designArtifactError("生成图片超过4MiB或内容为空，请简化要求后重试")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) == 0 || len(data) > designImageLimit {
		return nil, image.Config{}, "", designArtifactError("图片数据无效或超过4MiB，请重试")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") || config.Width < 1 || config.Height < 1 || config.Width > 8192 || config.Height > 8192 || int64(config.Width)*int64(config.Height) > 4_500_000 {
		return nil, image.Config{}, "", designArtifactError("生成图片格式或尺寸超限，请重试")
	}
	// Header validation alone accepts truncated JPEG/PNG files. Decode only after
	// checking both bytes and pixel count to keep malformed images off disk.
	decoded, actual, err := image.Decode(bytes.NewReader(data))
	if err != nil || actual != format || decoded.Bounds().Dx() != config.Width || decoded.Bounds().Dy() != config.Height {
		return nil, image.Config{}, "", designArtifactError("生成图片不完整或已损坏，请重试")
	}
	return data, config, format, nil
}
