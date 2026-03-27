package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
)

func TestDefaultUsePathStyleForEndpoint(t *testing.T) {
	tests := []struct {
		endpoint string
		wantPath bool
	}{
		{"https://oss-cn-hangzhou.aliyuncs.com", false},
		{"https://oss-cn-hangzhou-internal.aliyuncs.com", false},
		{"https://s3.cn-south-1.qiniucs.com", true},
		{"https://minio.example.com:9000", true},
		{"", true},
		{"not-a-url", true},
	}
	for _, tc := range tests {
		got := defaultUsePathStyleForEndpoint(tc.endpoint)
		if got != tc.wantPath {
			t.Errorf("defaultUsePathStyleForEndpoint(%q) = %v, want %v", tc.endpoint, got, tc.wantPath)
		}
	}
}

func TestPickS3AddressingModeEnv(t *testing.T) {
	t.Cleanup(func() {
		_ = os.Unsetenv(S3AddressingStyleKey)
		_ = os.Unsetenv(S3UsePathStyleKey)
	})
	_ = os.Unsetenv(S3AddressingStyleKey)
	_ = os.Unsetenv(S3UsePathStyleKey)

	if m := pickS3AddressingMode(nil); m != "auto" {
		t.Fatalf("pick nil extra: got %q", m)
	}
	_ = os.Setenv(S3AddressingStyleKey, "virtual")
	if m := pickS3AddressingMode(nil); m != "virtual" {
		t.Fatalf("env ADDRESSING virtual: got %q", m)
	}
	_ = os.Unsetenv(S3AddressingStyleKey)
	_ = os.Setenv(S3UsePathStyleKey, "false")
	if m := pickS3AddressingMode(nil); m != "virtual" {
		t.Fatalf("env USE_PATH_STYLE false: got %q", m)
	}
}

func TestNormalizeS3EndpointStripDuplicateBucket(t *testing.T) {
	tests := []struct {
		endpoint string
		bucket   string
		want     string
	}{
		{
			"https://qxerp2025.s3.oss-cn-hangzhou.aliyuncs.com",
			"qxerp2025",
			"https://s3.oss-cn-hangzhou.aliyuncs.com",
		},
		{
			"https://qxerp2025.oss-cn-hangzhou.aliyuncs.com",
			"qxerp2025",
			"https://oss-cn-hangzhou.aliyuncs.com",
		},
		{
			"https://s3.cn-south-1.qiniucs.com",
			"mybucket",
			"https://s3.cn-south-1.qiniucs.com",
		},
		{
			"https://minio.example.com:9000",
			"mybucket",
			"https://minio.example.com:9000",
		},
		{
			"https://mybucket.s3.oss-cn-hangzhou.aliyuncs.com:443",
			"mybucket",
			"https://s3.oss-cn-hangzhou.aliyuncs.com:443",
		},
	}
	for _, tc := range tests {
		got := normalizeS3EndpointStripDuplicateBucket(tc.endpoint, tc.bucket)
		if got != tc.want {
			t.Errorf("normalizeS3EndpointStripDuplicateBucket(%q, %q) = %q, want %q", tc.endpoint, tc.bucket, got, tc.want)
		}
	}
}

func TestNormalizeS3EndpointForPutObject_Kodo(t *testing.T) {
	const bucket = "kodobucket"
	in := "https://" + bucket + ".s3.cn-south-1.qiniucs.com"
	want := "https://s3.cn-south-1.qiniucs.com"
	got := normalizeS3EndpointForPutObject(in, bucket)
	if got != want {
		t.Fatalf("normalizeS3EndpointForPutObject(%q) = %q, want %q", in, got, want)
	}
}

func TestS3UsePathStyleFromResolvedMode(t *testing.T) {
	if !s3UsePathStyleFromResolvedMode("path", "https://oss-cn-hangzhou.aliyuncs.com") {
		t.Fatal("path mode must force path style")
	}
	if s3UsePathStyleFromResolvedMode("virtual", "https://minio.local") {
		t.Fatal("virtual mode must force virtual-hosted")
	}
	if s3UsePathStyleFromResolvedMode("auto", "https://oss-cn-hangzhou.aliyuncs.com") {
		t.Fatal("auto + aliyuncs must be virtual-hosted (UsePathStyle false)")
	}
	if !s3UsePathStyleFromResolvedMode("auto", "https://minio.local") {
		t.Fatal("auto + non-aliyuncs defaults to path style")
	}
}

func TestCanonicalS3ExtraKeyFromHeaderName(t *testing.T) {
	tests := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"S3-Bucket-Name", "S3_BUCKET_NAME", true},
		{"s3-bucket-name", "S3_BUCKET_NAME", true},
		{"S3_BUCKET_NAME", "S3_BUCKET_NAME", true},
		{"S3-Dir", "S3_DIR", true},
		{"Authorization", "", false},
		{"X-Custom", "", false},
	}
	for _, tc := range tests {
		got, ok := canonicalS3ExtraKeyFromHeaderName(tc.in)
		if ok != tc.wantOK || (tc.wantOK && got != tc.want) {
			t.Errorf("canonicalS3ExtraKeyFromHeaderName(%q) = (%q,%v), want (%q,%v)", tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestIsS3ImageExtraHeader(t *testing.T) {
	if !IsS3ImageExtraHeader("S3-Secret-Access-Key") {
		t.Fatal("expected S3 secret header recognized")
	}
	if IsS3ImageExtraHeader("X-Custom-Header") {
		t.Fatal("non-S3 header should be false")
	}
}

func mustMarshalJSONString(t *testing.T, s string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestEffectiveS3ImageExtraHeadersFillMissingOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Request.Header.Set("S3-Bucket-Name", "from-header")
	c.Request.Header.Set("S3-Dir", "from-header-dir")

	extra := effectiveS3ImageExtra(c, nil, nil)
	if got := string(extra["S3_BUCKET_NAME"]); got != `"from-header"` {
		t.Fatalf("S3_BUCKET_NAME: got %s want \"from-header\"", got)
	}
	if got := string(extra["S3_DIR"]); got != `"from-header-dir"` {
		t.Fatalf("S3_DIR: got %s", got)
	}

	img := &dto.ImageRequest{
		Extra: map[string]json.RawMessage{
			"S3_BUCKET_NAME": mustMarshalJSONString(t, "from-body"),
		},
	}
	merged := effectiveS3ImageExtra(c, nil, img)
	if got := string(merged["S3_BUCKET_NAME"]); got != `"from-body"` {
		t.Fatalf("body must win bucket: got %s", got)
	}
	if got := string(merged["S3_DIR"]); got != `"from-header-dir"` {
		t.Fatalf("header should fill S3_DIR: got %s", got)
	}
}

func TestEffectiveS3ImageExtraBodyWinsOverHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Request.Header.Set("S3-Region", "cn-from-header")
	img := &dto.ImageRequest{
		Extra: map[string]json.RawMessage{
			"S3_REGION": mustMarshalJSONString(t, "cn-from-body"),
		},
	}
	merged := effectiveS3ImageExtra(c, nil, img)
	if got := string(merged["S3_REGION"]); got != `"cn-from-body"` {
		t.Fatalf("S3_REGION: got %s", got)
	}
}
