package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tigerowo/infinite-canvas/model"
	"github.com/tigerowo/infinite-canvas/service"
)

const (
	defaultFanrenImageJobPollSeconds = 5
	defaultFanrenImageJobTimeout     = 30 * time.Minute
	maxFanrenImageJobN               = 4
)

type fanrenImageJobAsset struct {
	ProxyURL     string `json:"proxy_url"`
	ThumbnailURL string `json:"thumbnail_url"`
	URL          string `json:"url"`
}

type fanrenImageJob struct {
	ID           any                   `json:"id"`
	Status       string                `json:"status"`
	Progress     int                   `json:"progress"`
	ErrorMessage string                `json:"error_message"`
	Assets       []fanrenImageJobAsset `json:"assets"`
}

type fanrenImageJobResponse struct {
	Job     fanrenImageJob `json:"job"`
	Code    any            `json:"code"`
	Message string         `json:"message"`
	Msg     string         `json:"msg"`
	Error   any            `json:"error"`
}

func runFanrenImageTask(task model.CanvasImageTask, user model.AuthUser, body []byte, contentType string, channelID string, userChannelID string) {
	channel, resolvedUserChannelID, err := selectAIRequestChannel(user, task.Model, channelID, userChannelID)
	if err != nil {
		saveFailedCanvasImageTask(task, err.Error(), "")
		return
	}

	requestBody, err := normalizeFanrenImageJobBody(body, contentType, task.Model, task.Prompt)
	if err != nil {
		saveFailedCanvasImageTask(task, err.Error(), "")
		return
	}

	credits := 0
	if resolvedUserChannelID == "" {
		credits, err = service.ModelCost(task.Model)
		if err != nil {
			saveFailedCanvasImageTask(task, "读取图片模型成本失败", err.Error())
			return
		}
		if credits > 0 {
			if err := service.ConsumeUserCredits(user.ID, task.Model, credits, "/images/jobs"); err != nil {
				saveFailedCanvasImageTask(task, err.Error(), "")
				return
			}
		}
	}
	succeeded := false
	defer func() {
		if !succeeded && credits > 0 {
			if err := service.RefundUserCredits(user.ID, task.Model, credits, "/images/jobs"); err != nil {
				// The task failure remains visible even if a secondary refund fails.
				saveFailedCanvasImageTask(task, task.Error, fmt.Sprintf("%s; 退款失败: %v", task.ErrorDetail, err))
			}
		}
	}()

	task.Status = "processing"
	task.Progress = 10
	task.StartedAt = taskTime()
	task.RequestBody = summarizeAIRequest(requestBody, "application/json")
	task, _ = service.SaveCanvasImageTask(task)

	submitPayload, submitStatus, err := callFanrenImageJob(channel, http.MethodPost, "/images/jobs", requestBody)
	saveAIProxyLog(aiLogContext{
		StartedAt:       time.Now(),
		Endpoint:        "/images/jobs",
		Method:          http.MethodPost,
		Model:           task.Model,
		Channel:         channel,
		UserID:          user.ID,
		UserDisplayName: user.DisplayName,
		Credits:         credits,
		RequestBody:     task.RequestBody,
	}, submitStatus, string(submitPayload), "")
	if err != nil {
		task.Error = err.Error()
		saveFailedCanvasImageTask(task, task.Error, task.Error)
		return
	}
	if submitStatus >= http.StatusBadRequest {
		message := readFanrenImageJobError(submitPayload, submitStatus)
		saveFailedCanvasImageTask(task, message, string(submitPayload))
		return
	}

	job, err := parseFanrenImageJobResponse(submitPayload)
	if err != nil {
		saveFailedCanvasImageTask(task, err.Error(), string(submitPayload))
		return
	}
	if job.ID == "" {
		saveFailedCanvasImageTask(task, "图片任务没有返回任务 ID", string(submitPayload))
		return
	}
	task.ResponseBody = string(submitPayload)
	task.Progress = fanrenImageJobProgress(job)
	task, _ = service.SaveCanvasImageTask(task)
	if job.Status == "succeeded" {
		if !completeFanrenImageTask(&task, channel, job, submitPayload) {
			saveFailedCanvasImageTask(task, "图片任务完成但没有返回图片地址", string(submitPayload))
			return
		}
		succeeded = true
		return
	}
	if job.Status == "failed" {
		saveFailedCanvasImageTask(task, firstNonEmpty(job.Error, "图片任务生成失败"), string(submitPayload))
		return
	}

	pollEvery, timeout := fanrenImageJobPollConfig()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(pollEvery)
		pollPath := "/images/jobs/" + url.PathEscape(job.ID)
		pollPayload, pollStatus, pollErr := callFanrenImageJob(channel, http.MethodGet, pollPath, nil)
		saveAIProxyLog(aiLogContext{
			StartedAt:       time.Now(),
			Endpoint:        pollPath,
			Method:          http.MethodGet,
			Model:           task.Model,
			Channel:         channel,
			UserID:          user.ID,
			UserDisplayName: user.DisplayName,
			Credits:         credits,
			RequestBody:     fmt.Sprintf(`{"task_id":%q}`, job.ID),
		}, pollStatus, string(pollPayload), "")
		if pollErr != nil {
			saveFailedCanvasImageTask(task, pollErr.Error(), pollErr.Error())
			return
		}
		if pollStatus >= http.StatusBadRequest {
			saveFailedCanvasImageTask(task, readFanrenImageJobError(pollPayload, pollStatus), string(pollPayload))
			return
		}
		job, err = parseFanrenImageJobResponse(pollPayload)
		if err != nil {
			saveFailedCanvasImageTask(task, err.Error(), string(pollPayload))
			return
		}
		task.Progress = fanrenImageJobProgress(job)
		task.Status = "processing"
		task.ResponseBody = string(pollPayload)
		task, _ = service.SaveCanvasImageTask(task)
		if job.Status == "succeeded" {
			if !completeFanrenImageTask(&task, channel, job, pollPayload) {
				saveFailedCanvasImageTask(task, "图片任务完成但没有返回图片地址", string(pollPayload))
				return
			}
			succeeded = true
			return
		}
		if job.Status == "failed" {
			saveFailedCanvasImageTask(task, firstNonEmpty(job.Error, "图片任务生成失败"), string(pollPayload))
			return
		}
	}

	saveFailedCanvasImageTask(task, "图片任务等待上游结果超时", fmt.Sprintf("poll timeout after %s", timeout))
}

func normalizeFanrenImageJobBody(body []byte, contentType string, modelName string, prompt string) ([]byte, error) {
	if !strings.Contains(strings.ToLower(contentType), "json") {
		return nil, errors.New("Fanren 图片异步任务仅接受 JSON 请求")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("图片任务参数格式错误")
	}
	if strings.TrimSpace(fmt.Sprint(payload["model"])) == "" {
		payload["model"] = modelName
	}
	if strings.TrimSpace(fmt.Sprint(payload["prompt"])) == "" {
		payload["prompt"] = prompt
	}
	if strings.TrimSpace(fmt.Sprint(payload["prompt"])) == "" {
		return nil, errors.New("图片提示词不能为空")
	}
	if n := fanrenImageJobInt(payload["n"]); n > maxFanrenImageJobN {
		return nil, fmt.Errorf("图片任务最多支持 %d 张输出", maxFanrenImageJobN)
	} else if n > 0 {
		payload["n"] = n
	} else {
		payload["n"] = 1
	}
	// Fanren's job endpoint returns its own task state; streaming flags would
	// make the request ambiguous and are intentionally omitted.
	delete(payload, "stream")
	delete(payload, "partial_images")
	delete(payload, "response_format")
	normalized, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.New("图片任务参数序列化失败")
	}
	return normalized, nil
}

func callFanrenImageJob(channel model.ModelChannel, method string, path string, body []byte) ([]byte, int, error) {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequest(method, service.BuildModelChannelURL(channel, path), reader)
	if err != nil {
		return nil, 0, err
	}
	service.SetModelChannelAuthHeader(request, channel)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	return doAIRequest(request, channel)
}

type parsedFanrenImageJob struct {
	ID       string
	Status   string
	Progress int
	Assets   []fanrenImageJobAsset
	Error    string
}

func parseFanrenImageJobResponse(payload []byte) (parsedFanrenImageJob, error) {
	var response fanrenImageJobResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		return parsedFanrenImageJob{}, errors.New("图片任务响应格式错误")
	}
	result := parsedFanrenImageJob{
		ID:       fanrenImageJobID(response.Job.ID),
		Status:   normalizeFanrenImageJobStatus(response.Job.Status),
		Progress: response.Job.Progress,
		Assets:   response.Job.Assets,
		Error:    firstNonEmpty(response.Job.ErrorMessage, fanrenImageJobError(response.Error), response.Message, response.Msg),
	}
	if result.Status == "" && len(result.Assets) > 0 {
		result.Status = "succeeded"
	} else if result.Status == "" {
		result.Status = "queued"
	}
	return result, nil
}

func normalizeFanrenImageJobStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "succeeded", "completed", "success", "done":
		return "succeeded"
	case "failed", "failure", "error", "cancelled", "canceled":
		return "failed"
	case "running", "processing", "in_progress":
		return "processing"
	case "queued", "pending", "created":
		return "queued"
	case "":
		return ""
	default:
		return "processing"
	}
}

func fanrenImageJobID(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case json.Number:
		return typed.String()
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func fanrenImageJobError(value any) string {
	if value == nil {
		return ""
	}
	if typed, ok := value.(map[string]any); ok {
		return firstNonEmpty(fmt.Sprint(typed["message"]), fmt.Sprint(typed["msg"]), fmt.Sprint(typed["code"]))
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func readFanrenImageJobError(payload []byte, status int) string {
	parsed, err := parseFanrenImageJobResponse(payload)
	if err == nil && parsed.Error != "" {
		return parsed.Error
	}
	if status > 0 {
		return fmt.Sprintf("Fanren 图片任务请求失败（HTTP %d）", status)
	}
	return "Fanren 图片任务请求失败"
}

func fanrenImageJobProgress(job parsedFanrenImageJob) int {
	if job.Progress > 0 && job.Progress < 100 {
		return job.Progress
	}
	if job.Status == "succeeded" {
		return 100
	}
	if job.Status == "processing" {
		return 50
	}
	return 20
}

func completeFanrenImageTask(task *model.CanvasImageTask, channel model.ModelChannel, job parsedFanrenImageJob, payload []byte) bool {
	urls := make([]string, 0, len(job.Assets))
	seen := map[string]bool{}
	for _, asset := range job.Assets {
		value := firstNonEmpty(asset.ProxyURL, asset.URL, asset.ThumbnailURL)
		value = absoluteFanrenImageAssetURL(channel.BaseURL, value)
		if value != "" && !seen[value] {
			urls = append(urls, value)
			seen[value] = true
		}
	}
	if len(urls) == 0 {
		return false
	}
	task.Status = "completed"
	task.Progress = 100
	task.CompletedAt = taskTime()
	task.ResponseBody = string(payload)
	task.ImageURL = urls[0]
	task.ImageURLs = urls
	task.StorageKey = ""
	task.MimeType = "image/png"
	task.Bytes = 0
	task.Error = ""
	task.ErrorDetail = ""
	_, _ = service.SaveCanvasImageTask(*task)
	return true
}

func absoluteFanrenImageAssetURL(baseURL string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.IsAbs() {
		return value
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	for _, suffix := range []string{"/v1", "/api/v3", "/api/paas/v4", "/api/plan/v3"} {
		if strings.HasSuffix(strings.ToLower(base), suffix) {
			base = strings.TrimRight(base[:len(base)-len(suffix)], "/")
			break
		}
	}
	return base + "/" + strings.TrimLeft(value, "/")
}

func fanrenImageJobInt(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case json.Number:
		number, _ := typed.Int64()
		return int(number)
	case int:
		return typed
	case string:
		number, _ := strconv.Atoi(strings.TrimSpace(typed))
		return number
	default:
		return 0
	}
}

func fanrenImageJobPollConfig() (time.Duration, time.Duration) {
	pollSeconds := boundedFanrenEnvInt("FANREN_IMAGE_JOB_POLL_SECONDS", defaultFanrenImageJobPollSeconds, 1, 60)
	timeoutSeconds := boundedFanrenEnvInt("FANREN_IMAGE_JOB_TIMEOUT_SECONDS", int(defaultFanrenImageJobTimeout/time.Second), 60, 2*60*60)
	return time.Duration(pollSeconds) * time.Second, time.Duration(timeoutSeconds) * time.Second
}

func boundedFanrenEnvInt(name string, fallback int, min int, max int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil || value < min {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}
