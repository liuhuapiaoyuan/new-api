package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/s3_image_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/gin-gonic/gin"
)

// S3ImageConfigKeys 与常见环境变量名一致；仅当请求中出现其中至少一个键时才触发上传（不含 S3_DIR）。
var S3ImageConfigKeys = []string{
	"S3_BUCKET_NAME",
	"S3_ACCESS_KEY_ID",
	"S3_SECRET_ACCESS_KEY",
	"S3_ENDPOINT",
	"S3_REGION",
	"S3_CDN",
}

// S3DirKey 可选对象键前缀（子目录），与 S3_BUCKET_NAME 等一并从 JSON/multipart 剥离。
const S3DirKey = "S3_DIR"

// S3FlatObjectKeyExtra 为 true 时用 __ 代替路径分隔，对象键为单层路径（部分 CDN 对多级路径回源异常时可缓解）。
const S3FlatObjectKeyExtra = "S3_FLAT_OBJECT_KEY"

func s3StripKeys() []string {
	return append(append(append([]string{}, S3ImageConfigKeys...), S3DirKey), S3FlatObjectKeyExtra)
}

// IsS3ImageExtraKey 用于 multipart 转发上游时过滤字段。
func IsS3ImageExtraKey(k string) bool {
	for _, x := range s3StripKeys() {
		if k == x {
			return true
		}
	}
	return false
}

// PopulateS3ExtraFromMultipart 将 multipart 表单中的 S3_* 字段写入 imageRequest.Extra，供后续 CDN 上传判断。
func PopulateS3ExtraFromMultipart(imageRequest *dto.ImageRequest, formData map[string][]string) {
	if imageRequest == nil || len(formData) == 0 {
		return
	}
	if imageRequest.Extra == nil {
		imageRequest.Extra = make(map[string]json.RawMessage)
	}
	for _, key := range s3StripKeys() {
		values := formData[key]
		if len(values) == 0 || values[0] == "" {
			continue
		}
		raw, err := json.Marshal(values[0])
		if err != nil {
			continue
		}
		imageRequest.Extra[key] = raw
	}
}

// StripS3KeysFromJSON 从顶层 JSON object 移除 S3_* 键，供上游请求体使用；返回值 extra 供 RelayInfo / 合并逻辑使用。
func StripS3KeysFromJSON(raw []byte) (stripped []byte, extra map[string]json.RawMessage, err error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return raw, nil, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, nil, err
	}
	if len(m) == 0 {
		return raw, nil, nil
	}
	extra = make(map[string]json.RawMessage)
	changed := false
	for _, key := range s3StripKeys() {
		if v, ok := m[key]; ok {
			extra[key] = v
			delete(m, key)
			changed = true
		}
	}
	if !changed {
		return raw, nil, nil
	}
	stripped, err = json.Marshal(m)
	if err != nil {
		return nil, extra, err
	}
	if len(extra) == 0 {
		extra = nil
	}
	return stripped, extra, nil
}

// effectiveS3ImageExtra 合并顺序：ImageRequest.Extra 覆盖 RelayInfo / Gin 上下文的 S3 同名字段（与 mergeS3Field 的「请求优先」一致）。
func effectiveS3ImageExtra(c *gin.Context, info *relaycommon.RelayInfo, imgReq *dto.ImageRequest) map[string]json.RawMessage {
	var base map[string]json.RawMessage
	if info != nil && len(info.S3ImageExtra) > 0 {
		base = info.S3ImageExtra
	}
	if len(base) == 0 && c != nil {
		if v, ok := common.GetContextKeyType[map[string]json.RawMessage](c, constant.ContextKeyRelayS3ImageExtra); ok && len(v) > 0 {
			base = v
		}
	}
	var out map[string]json.RawMessage
	if len(base) > 0 {
		out = make(map[string]json.RawMessage, len(base)+8)
		for k, v := range base {
			out[k] = v
		}
	}
	if imgReq != nil && len(imgReq.Extra) > 0 {
		if out == nil {
			out = make(map[string]json.RawMessage, len(imgReq.Extra))
		}
		for k, v := range imgReq.Extra {
			out[k] = v
		}
	}
	return out
}

func clientRequestedS3FromExtra(extra map[string]json.RawMessage) bool {
	if extra == nil {
		return false
	}
	for _, key := range S3ImageConfigKeys {
		if raw, ok := extra[key]; ok && len(raw) > 0 {
			var s string
			if json.Unmarshal(raw, &s) == nil && strings.TrimSpace(s) != "" {
				return true
			}
		}
	}
	return false
}

// mergeS3Field 合并顺序：请求 Extra > 数据库 s3_image_setting > 环境变量
func mergeS3Field(extra map[string]json.RawMessage, envKey string, dbVal string) string {
	if extra != nil {
		if raw, ok := extra[envKey]; ok && len(raw) > 0 {
			var s string
			if json.Unmarshal(raw, &s) == nil && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	if strings.TrimSpace(dbVal) != "" {
		return strings.TrimSpace(dbVal)
	}
	return strings.TrimSpace(os.Getenv(envKey))
}

func dbFieldForEnvKey(set *s3_image_setting.S3ImageSetting, envKey string) string {
	if set == nil {
		return ""
	}
	switch envKey {
	case "S3_BUCKET_NAME":
		return set.Bucket
	case "S3_REGION":
		return set.Region
	case "S3_ENDPOINT":
		return set.Endpoint
	case "S3_ACCESS_KEY_ID":
		return set.AccessKeyId
	case "S3_SECRET_ACCESS_KEY":
		return set.Secret
	case "S3_CDN":
		return set.Cdn
	case "S3_DIR":
		return set.Dir
	default:
		return ""
	}
}

type s3ImageResolvedConfig struct {
	Bucket   string
	Region   string
	Endpoint string
	AK       string
	SK       string
	CDN      string
}

func resolveS3ImageConfig(extra map[string]json.RawMessage) (*s3ImageResolvedConfig, error) {
	set := s3_image_setting.GetS3ImageSetting()
	cfg := &s3ImageResolvedConfig{
		Bucket:   mergeS3Field(extra, "S3_BUCKET_NAME", dbFieldForEnvKey(set, "S3_BUCKET_NAME")),
		Region:   mergeS3Field(extra, "S3_REGION", dbFieldForEnvKey(set, "S3_REGION")),
		Endpoint: mergeS3Field(extra, "S3_ENDPOINT", dbFieldForEnvKey(set, "S3_ENDPOINT")),
		AK:       mergeS3Field(extra, "S3_ACCESS_KEY_ID", dbFieldForEnvKey(set, "S3_ACCESS_KEY_ID")),
		SK:       mergeS3Field(extra, "S3_SECRET_ACCESS_KEY", dbFieldForEnvKey(set, "S3_SECRET_ACCESS_KEY")),
		CDN:      mergeS3Field(extra, "S3_CDN", dbFieldForEnvKey(set, "S3_CDN")),
	}
	if cfg.Bucket == "" || cfg.Region == "" || cfg.Endpoint == "" || cfg.AK == "" || cfg.SK == "" || cfg.CDN == "" {
		return nil, fmt.Errorf("incomplete S3 configuration: need S3_BUCKET_NAME, S3_REGION, S3_ENDPOINT, S3_ACCESS_KEY_ID, S3_SECRET_ACCESS_KEY, S3_CDN (request, console, and/or environment)")
	}
	return cfg, nil
}

// S3MergedConfigComplete 用于控制台保存前校验（extra 可为 nil）
func S3MergedConfigComplete(extra map[string]json.RawMessage) bool {
	_, err := resolveS3ImageConfig(extra)
	return err == nil
}

func extForImageContentType(ct string) string {
	switch strings.ToLower(ct) {
	case "image/png":
		return "png"
	case "image/jpeg", "image/jpg":
		return "jpg"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	default:
		return "png"
	}

}

func sniffImageContentType(data []byte) string {
	if len(data) > 4 && data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "image/png"
	}
	if len(data) > 2 && data[0] == 0xFF && data[1] == 0xD8 {
		return "image/jpeg"
	}
	if len(data) > 12 && data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 {
		return "image/webp"
	}
	return "image/png"
}

// normalizeKodoS3EndpointForPathStyle 将七牛虚拟主机式 endpoint（<bucket>.s3.<region>.qiniucs.com）
// 规范为 path-style 使用的 s3.<region>.qiniucs.com，避免与 UsePathStyle+Bucket 组合时对象键错位。
func normalizeKodoS3EndpointForPathStyle(endpoint, bucket string) string {
	endpoint = strings.TrimSpace(endpoint)
	bucket = strings.TrimSpace(bucket)
	if endpoint == "" || bucket == "" {
		return endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	host := strings.ToLower(u.Hostname())
	b := strings.ToLower(bucket)
	if !strings.Contains(host, ".qiniucs.com") {
		return endpoint
	}
	prefix := b + ".s3."
	if !strings.HasPrefix(host, prefix) {
		return endpoint
	}
	rest := strings.TrimPrefix(host, prefix)
	u.Host = "s3." + rest
	return u.String()
}

func putObjectToS3(ctx context.Context, cfg *s3ImageResolvedConfig, key string, body []byte, contentType string) error {
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AK, cfg.SK, "")),
	)
	if err != nil {
		return fmt.Errorf("aws config: %w", err)
	}
	endpoint := normalizeKodoS3EndpointForPathStyle(cfg.Endpoint, cfg.Bucket)
	if endpoint == "" {
		endpoint = cfg.Endpoint
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(cfg.Bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(body),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("s3 put object: %w", err)
	}
	return nil
}

func cdnURLForKey(cdnBase, objectKey string) string {
	cdnBase = strings.TrimSpace(cdnBase)
	objectKey = strings.Trim(strings.TrimSpace(objectKey), "/")
	if cdnBase == "" {
		return objectKey
	}
	if objectKey == "" {
		return strings.TrimRight(cdnBase, "/")
	}
	segments := strings.Split(objectKey, "/")
	elems := make([]string, 0, len(segments))
	for _, s := range segments {
		if s != "" {
			elems = append(elems, s)
		}
	}
	base := strings.TrimRight(cdnBase, "/")
	out, err := url.JoinPath(base, elems...)
	if err != nil {
		return base + "/" + objectKey
	}
	return out
}

func sanitizeS3DirPrefix(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\\", "/")
	for strings.Contains(s, "..") {
		s = strings.ReplaceAll(s, "..", "")
	}
	s = strings.Trim(s, "/")
	if s == "" {
		return ""
	}
	parts := strings.Split(s, "/")
	var clean []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "." {
			continue
		}
		clean = append(clean, p)
	}
	return strings.Join(clean, "/")
}

// mergeS3DirPrefix 合并顺序与 mergeS3Field 一致：请求 Extra > 数据库 s3_image_setting.dir > 环境变量 S3_DIR。
func mergeS3DirPrefix(extra map[string]json.RawMessage) string {
	set := s3_image_setting.GetS3ImageSetting()
	return sanitizeS3DirPrefix(mergeS3Field(extra, S3DirKey, dbFieldForEnvKey(set, S3DirKey)))
}

// mergeFlatObjectKey 为 true 时对象键使用 __ 连接（单层 URL 路径），环境变量 S3_FLAT_OBJECT_KEY 或请求 S3_FLAT_OBJECT_KEY。
func mergeFlatObjectKey(extra map[string]json.RawMessage) bool {
	if v := strings.TrimSpace(os.Getenv("S3_FLAT_OBJECT_KEY")); v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes") {
		return true
	}
	if extra == nil {
		return false
	}
	raw, ok := extra[S3FlatObjectKeyExtra]
	if !ok || len(raw) == 0 {
		return false
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		s = strings.TrimSpace(s)
		if s == "1" || strings.EqualFold(s, "true") || strings.EqualFold(s, "yes") {
			return true
		}
	}
	var b bool
	if json.Unmarshal(raw, &b) == nil && b {
		return true
	}
	return false
}

func buildS3ObjectKey(dirPrefix, fileName string, flat bool) string {
	dirPrefix = sanitizeS3DirPrefix(dirPrefix)
	fileName = strings.TrimLeft(strings.TrimSpace(fileName), "/")
	if fileName == "" {
		fileName = uuid.New().String()
	}
	if dirPrefix == "" {
		return fileName
	}
	if flat {
		dirFlat := strings.ReplaceAll(dirPrefix, "/", "__")
		return dirFlat + "__" + fileName
	}
	return dirPrefix + "/" + fileName
}

func shouldRunS3ImageTransform(extra map[string]json.RawMessage) bool {
	if clientRequestedS3FromExtra(extra) {
		return true
	}
	if s3_image_setting.GetS3ImageSetting().Enabled {
		return true
	}
	return false
}

// TransformImageResponseBodyToS3IfConfigured 在以下情况上传并改写为 url：请求含 S3_*（OpenAI 图接口 ImageRequest.Extra 或 Gemini JSON 剥离后的 RelayInfo.S3ImageExtra），或控制台启用系统级 S3；否则返回原 body。
func TransformImageResponseBodyToS3IfConfigured(c *gin.Context, info *relaycommon.RelayInfo, body []byte) ([]byte, *types.NewAPIError) {
	if info == nil || len(body) == 0 {
		return body, nil
	}
	var imgReq *dto.ImageRequest
	if ir, ok := info.Request.(*dto.ImageRequest); ok && ir != nil {
		imgReq = ir
	}
	extra := effectiveS3ImageExtra(c, info, imgReq)
	if !shouldRunS3ImageTransform(extra) {
		return body, nil
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(body, &probe); err == nil {
		if _, hasErr := probe["error"]; hasErr {
			return body, nil
		}
	}
	resolved, err := resolveS3ImageConfig(extra)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	dirPrefix := mergeS3DirPrefix(extra)
	flatKey := mergeFlatObjectKey(extra)

	var imageResp dto.ImageResponse
	if err := common.Unmarshal(body, &imageResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	ctx := c.Request.Context()
	for i := range imageResp.Data {
		if imageResp.Data[i].Url != "" && imageResp.Data[i].B64Json == "" {
			continue
		}
		b64 := imageResp.Data[i].B64Json
		if b64 == "" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, types.NewErrorWithStatusCode(fmt.Errorf("decode image base64: %w", err), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		ct := sniffImageContentType(raw)
		ext := extForImageContentType(ct)
		objectKey := buildS3ObjectKey(dirPrefix, fmt.Sprintf("%s.%s", uuid.New().String(), ext), flatKey)
		if err := putObjectToS3(ctx, resolved, objectKey, raw, ct); err != nil {
			return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		imageResp.Data[i].Url = cdnURLForKey(resolved.CDN, objectKey)
		imageResp.Data[i].B64Json = ""
	}

	out, err := common.Marshal(imageResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	return out, nil
}

func decodeImageBase64Payload(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("empty")
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err == nil {
		return raw, nil
	}
	return base64.RawURLEncoding.DecodeString(s)
}

// TransformGeminiNativeChatResponseToS3IfConfigured 处理 Gemini 原生 JSON（candidates[].content.parts[].inlineData.data），上传后写入 url 并清空 data。
// 用于 gemini-*-image*:predict 等走 GeminiTextGenerationHandler 的路径（非 Imagen predictions 格式）。
func TransformGeminiNativeChatResponseToS3IfConfigured(c *gin.Context, info *relaycommon.RelayInfo, body []byte) ([]byte, *types.NewAPIError) {
	if info == nil || len(body) == 0 {
		return body, nil
	}
	var imgReq *dto.ImageRequest
	if ir, ok := info.Request.(*dto.ImageRequest); ok && ir != nil {
		imgReq = ir
	}
	extra := effectiveS3ImageExtra(c, info, imgReq)
	if !shouldRunS3ImageTransform(extra) {
		return body, nil
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(body, &probe); err == nil {
		if _, hasErr := probe["error"]; hasErr {
			return body, nil
		}
	}
	resolved, err := resolveS3ImageConfig(extra)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	dirPrefix := mergeS3DirPrefix(extra)
	flatKey := mergeFlatObjectKey(extra)

	var chatResp dto.GeminiChatResponse
	if err := common.Unmarshal(body, &chatResp); err != nil {
		return body, nil
	}

	ctx := c.Request.Context()
	changed := false
	for ci := range chatResp.Candidates {
		for pi := range chatResp.Candidates[ci].Content.Parts {
			part := &chatResp.Candidates[ci].Content.Parts[pi]
			id := part.InlineData
			if id == nil || id.Data == "" {
				continue
			}
			raw, err := decodeImageBase64Payload(id.Data)
			if err != nil {
				continue
			}
			ct := sniffImageContentType(raw)
			if id.MimeType != "" && !strings.HasPrefix(strings.ToLower(id.MimeType), "image/") {
				continue
			}
			if id.MimeType != "" {
				ct = id.MimeType
			}
			ext := extForImageContentType(ct)
			objectKey := buildS3ObjectKey(dirPrefix, fmt.Sprintf("%s.%s", uuid.New().String(), ext), flatKey)
			if err := putObjectToS3(ctx, resolved, objectKey, raw, ct); err != nil {
				return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			}
			id.Url = cdnURLForKey(resolved.CDN, objectKey)
			id.Data = ""
			changed = true
		}
	}
	if !changed {
		return body, nil
	}
	out, err := common.Marshal(chatResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	return out, nil
}
