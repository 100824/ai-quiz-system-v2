package v2

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type aiProvider struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Model      string `json:"model"`
	Configured bool   `json:"configured"`
	url        string
	key        string
}

const glmModel = "glm-5.3-flash"

func envFirst(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func (h *Handler) aiProviders() []aiProvider {
	dsURL := h.deepSeekURL
	if dsURL == "" {
		dsURL = "https://api.deepseek.com/chat/completions"
	}
	glmURL := h.glmURL
	if glmURL == "" {
		glmURL = "https://open.bigmodel.cn/api/paas/v4/chat/completions"
	}
	dsModel := envFirst("DEEPSEEK_MODEL")
	if dsModel == "" {
		dsModel = aiGuidanceModel
	}
	providers := []aiProvider{
		{ID: "deepseek", Name: "DeepSeek", Model: dsModel, url: dsURL, key: envFirst("DEEPSEEK_KEY", "DEEPSEEK_API_KEY")},
		{ID: "glm", Name: "GLM（智谱）", Model: glmModel, url: glmURL, key: envFirst("GLM_KEY", "GLM_API_KEY")},
	}
	for i := range providers {
		providers[i].Configured = providers[i].key != ""
	}
	return providers
}

func (r *Repository) GetPrimaryAIProvider() (string, error) {
	var provider string
	err := r.db.QueryRow(`SELECT value FROM app_settings WHERE key = 'primary_ai_provider'`).Scan(&provider)
	if errors.Is(err, sql.ErrNoRows) {
		return "deepseek", nil
	}
	if err != nil {
		return "", err
	}
	if provider != "deepseek" && provider != "glm" {
		return "", errors.New("主模型配置无效")
	}
	return provider, nil
}

func (r *Repository) SetPrimaryAIProvider(provider string) error {
	if provider != "deepseek" && provider != "glm" {
		return errors.New("不支持的模型厂商")
	}
	_, err := r.db.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES ('primary_ai_provider', ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`, provider)
	return err
}

func (h *Handler) HandleGetAISettings(w http.ResponseWriter, r *http.Request) {
	primary, err := h.repo.GetPrimaryAIProvider()
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"primaryProvider": primary, "providers": h.aiProviders()}})
}

func (h *Handler) HandleSetAISettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PrimaryProvider string `json:"primaryProvider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Error: "模型配置格式错误"})
		return
	}
	valid := false
	for _, provider := range h.aiProviders() {
		if provider.ID == body.PrimaryProvider && provider.Configured {
			valid = true
		}
	}
	if !valid {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Error: "所选厂商不存在或尚未配置密钥"})
		return
	}
	if err := h.repo.SetPrimaryAIProvider(body.PrimaryProvider); err != nil {
		h.writeError(w, err)
		return
	}
	h.HandleGetAISettings(w, r)
}

// Each attempt has its own deadline so a stalled primary leaves time for fallback.
func (h *Handler) streamAI(ctx context.Context, prompt string, history []AIChatMessage, onDelta func(string) error, onReset func() error) (string, string, error) {
	primary := "deepseek"
	if h.repo != nil {
		var err error
		primary, err = h.repo.GetPrimaryAIProvider()
		if err != nil {
			return "", "", err
		}
	}
	providers := h.aiProviders()
	if primary == "glm" {
		providers[0], providers[1] = providers[1], providers[0]
	}
	timeout := h.aiProviderTimeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	var failures []string
	for _, provider := range providers {
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		if !provider.Configured {
			failures = append(failures, provider.Name+"未配置密钥")
			continue
		}
		attemptCtx, cancel := context.WithTimeout(ctx, timeout)
		emitted, writeFailed := false, false
		answer, err := h.streamProvider(attemptCtx, provider, prompt, history, func(delta string) error {
			if onDelta == nil {
				return nil
			}
			emitted = true
			err := onDelta(delta)
			writeFailed = err != nil
			return err
		})
		cancel()
		if err == nil {
			return answer, provider.Model, nil
		}
		if ctx.Err() != nil {
			return "", "", ctx.Err()
		}
		if writeFailed {
			return "", "", err
		}
		log.Printf("AI provider %s failed: %v", provider.ID, err)
		failures = append(failures, provider.Name+"调用失败")
		if emitted {
			if onReset == nil {
				return "", "", err
			}
			if err := onReset(); err != nil {
				return "", "", err
			}
		}
	}
	return "", "", fmt.Errorf("AI服务暂不可用：%s，请稍后重试", strings.Join(failures, "；"))
}

func (h *Handler) streamAIToClient(ctx context.Context, w http.ResponseWriter, prompt string, history []AIChatMessage) (string, string, error) {
	return h.streamAI(ctx, prompt, history, func(delta string) error {
		return writeAIStreamEvent(w, "delta", map[string]string{"content": delta})
	}, func() error {
		return writeAIStreamEvent(w, "reset", map[string]string{"reason": "正在切换备用模型"})
	})
}

// Retained for existing internal callers and streaming parser tests.
func (h *Handler) streamDeepSeek(ctx context.Context, prompt string, history []AIChatMessage, onDelta func(string) error) (string, error) {
	answer, _, err := h.streamAI(ctx, prompt, history, onDelta, nil)
	return answer, err
}
