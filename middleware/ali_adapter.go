package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
)

// aliVideoRequest 用于解析 Ali 原生请求格式
type aliVideoRequest struct {
	Model      string                 `json:"model"`
	Input      map[string]interface{} `json:"input"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// AliRequestConvert 将阿里云原生视频 API 请求转换为统一格式
//
// 与 Kling/Jimeng 中间件的关键区别：
//   - 请求转换为统一格式供内部处理
//   - 设置 ali_passthrough 标志，使响应以 Ali 原生格式输出
//   - 保持与现有 Ali 视频任务计费逻辑的兼容性
func AliRequestConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 设置 Ali 透传标志（所有请求都需要）
		c.Set("ali_passthrough", true)

		// GET 请求：仅做路径重写，不做 body 解析
		if c.Request.Method == http.MethodGet {
			taskId := c.Param("task_id")
			c.Request.URL.Path = "/v1/video/generations/" + taskId
			c.Set("task_id", taskId)
			c.Set("relay_mode", relayconstant.RelayModeVideoFetchByID)
			c.Next()
			return
		}

		// POST 请求：解析 body、校验、转换
		var aliReq aliVideoRequest
		if err := common.UnmarshalBodyReusable(c, &aliReq); err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "Invalid request body")
			return
		}

		// 验证必填字段
		if aliReq.Model == "" {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "model field is required")
			return
		}

		// 提取 prompt
		prompt, _ := aliReq.Input["prompt"].(string)
		if strings.TrimSpace(prompt) == "" {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "input.prompt is required")
			return
		}

		// 构建统一格式请求
		unifiedReq := map[string]interface{}{
			"model":    aliReq.Model,
			"prompt":   prompt,
			"metadata": buildAliMetadata(aliReq),
		}

		// 检测是否有媒体输入（用于确定 action）
		hasMedia := detectAliMediaInput(aliReq.Input)

		jsonData, err := common.Marshal(unifiedReq)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "Failed to marshal request body")
			return
		}

		// 重写请求体和路径
		c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonData))
		c.Request.URL.Path = "/v1/video/generations"

		// 设置 action
		if !hasMedia {
			c.Set("action", constant.TaskActionTextGenerate)
		}

		// 缓存请求体供后续使用
		c.Set(common.KeyRequestBody, jsonData)

		c.Next()
	}
}

// buildAliMetadata 构建 metadata，保留原始 Ali 请求的所有字段
func buildAliMetadata(aliReq aliVideoRequest) map[string]interface{} {
	metadata := make(map[string]interface{})

	// 保留原始 input 和 parameters
	if aliReq.Input != nil {
		metadata["input"] = aliReq.Input
	}
	if aliReq.Parameters != nil {
		metadata["parameters"] = aliReq.Parameters
	}

	// 提取常用字段到顶层便于适配器处理

	// 图片类字段
	if v, ok := aliReq.Input["img_url"].(string); ok && v != "" {
		metadata["img_url"] = v
	}
	if v, ok := aliReq.Input["first_frame_url"].(string); ok && v != "" {
		metadata["first_frame_url"] = v
	}
	if v, ok := aliReq.Input["last_frame_url"].(string); ok && v != "" {
		metadata["last_frame_url"] = v
	}
	if v, ok := aliReq.Input["mask_image_url"].(string); ok && v != "" {
		metadata["mask_image_url"] = v
	}

	// 音频类字段
	if v, ok := aliReq.Input["audio_url"].(string); ok && v != "" {
		metadata["audio_url"] = v
	}

	// 视频类字段
	if v, ok := aliReq.Input["video_url"].(string); ok && v != "" {
		metadata["video_url"] = v
	}
	if v, ok := aliReq.Input["first_clip_url"].(string); ok && v != "" {
		metadata["first_clip_url"] = v
	}
	if v, ok := aliReq.Input["last_clip_url"].(string); ok && v != "" {
		metadata["last_clip_url"] = v
	}
	if v, ok := aliReq.Input["mask_video_url"].(string); ok && v != "" {
		metadata["mask_video_url"] = v
	}

	// 其他字段
	if v, ok := aliReq.Input["negative_prompt"].(string); ok && v != "" {
		metadata["negative_prompt"] = v
	}
	if v, ok := aliReq.Input["template"].(string); ok && v != "" {
		metadata["template"] = v
	}
	if v, ok := aliReq.Input["function"].(string); ok && v != "" {
		metadata["function"] = v
	}
	if v, ok := aliReq.Input["mask_frame_id"]; ok {
		metadata["mask_frame_id"] = v
	}

	// URL 数组类字段
	if urls, ok := aliReq.Input["ref_images_url"].([]interface{}); ok && len(urls) > 0 {
		metadata["ref_images_url"] = urls
	}

	// 提取 parameters 字段
	if aliReq.Parameters != nil {
		if v, ok := aliReq.Parameters["resolution"].(string); ok {
			metadata["resolution"] = v
		}
		if v, ok := aliReq.Parameters["duration"]; ok {
			metadata["duration"] = v
		}
		if v, ok := aliReq.Parameters["size"].(string); ok {
			metadata["size"] = v
		}
		if v, ok := aliReq.Parameters["prompt_extend"]; ok {
			metadata["prompt_extend"] = v
		}
		if v, ok := aliReq.Parameters["watermark"]; ok {
			metadata["watermark"] = v
		}
		if v, ok := aliReq.Parameters["seed"]; ok {
			metadata["seed"] = v
		}
		if v, ok := aliReq.Parameters["ratio"].(string); ok {
			metadata["ratio"] = v
		}
	}

	// 处理 media 数组（新格式）
	if media, ok := aliReq.Input["media"].([]interface{}); ok && len(media) > 0 {
		metadata["media"] = media
	}

	// 处理 reference_urls
	if urls, ok := aliReq.Input["reference_urls"].([]interface{}); ok && len(urls) > 0 {
		metadata["reference_urls"] = urls
	}

	return metadata
}

// detectAliMediaInput 检测 Ali 请求中是否包含任何媒体输入（图片、音频、视频等）
// 覆盖所有已知的 Ali 模型媒体字段：
//   - 新格式 (wan2.7+, happyhorse): input.media 数组，type 包括 first_frame/last_frame/reference_image/reference_video/driving_audio/video/first_clip 等
//   - 旧格式 (wan2.6/wan2.2/wanx2.1): input 中的扁平字段如 img_url/audio_url/reference_urls/first_frame_url/last_frame_url/video_url/first_clip_url/last_clip_url/ref_images_url/mask_image_url/mask_video_url
func detectAliMediaInput(input map[string]interface{}) bool {
	if input == nil {
		return false
	}

	// ── 新格式：media 数组 (wan2.7+, happyhorse) ──
	// 只要 media 数组非空即视为有媒体输入，不限制具体 type
	if media, ok := input["media"].([]interface{}); ok && len(media) > 0 {
		for _, m := range media {
			if mediaItem, ok := m.(map[string]interface{}); ok {
				url, _ := mediaItem["url"].(string)
				if url != "" {
					return true
				}
			}
		}
	}

	// ── 旧格式：扁平字段 (wan2.6/wan2.2/wanx2.1) ──

	// 图片类字段
	imageFields := []string{
		"img_url",         // wan2.6-i2v-flash, wan2.6-i2v
		"first_frame_url", // wan2.2-kf2v-flash, wan2.7 (flat fallback)
		"last_frame_url",  // wan2.2-kf2v-flash
		"mask_image_url",  // wanx2.1-vace-plus
	}
	for _, field := range imageFields {
		if v, ok := input[field].(string); ok && v != "" {
			return true
		}
	}

	// URL 数组类字段
	arrayFields := []string{
		"reference_urls", // wan2.6-r2v-flash
		"ref_images_url", // wanx2.1-vace-plus
	}
	for _, field := range arrayFields {
		if urls, ok := input[field].([]interface{}); ok && len(urls) > 0 {
			return true
		}
	}

	// 音频类字段
	audioFields := []string{
		"audio_url", // wan2.6-i2v-flash, wan2.6-t2v
	}
	for _, field := range audioFields {
		if v, ok := input[field].(string); ok && v != "" {
			return true
		}
	}

	// 视频类字段
	videoFields := []string{
		"video_url",      // wanx2.1-vace-plus
		"first_clip_url", // wanx2.1-vace-plus
		"last_clip_url",  // wanx2.1-vace-plus
		"mask_video_url", // wanx2.1-vace-plus
	}
	for _, field := range videoFields {
		if v, ok := input[field].(string); ok && v != "" {
			return true
		}
	}

	return false
}
