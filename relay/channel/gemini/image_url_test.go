package gemini

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	service.InitHttpClient()
	if constant.MaxFileDownloadMB <= 0 {
		constant.MaxFileDownloadMB = 64
	}
}

func withSSRFDisabled(t *testing.T) {
	t.Helper()
	fs := system_setting.GetFetchSetting()
	prev := fs.EnableSSRFProtection
	fs.EnableSSRFProtection = false
	t.Cleanup(func() {
		fs.EnableSSRFProtection = prev
	})
}

func TestIsGeminiNativeFileURI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		uri  string
		want bool
	}{
		{"https://www.youtube.com/watch?v=abc", true},
		{"https://youtu.be/abc", true},
		{"gs://bucket/object.png", true},
		{"https://generativelanguage.googleapis.com/v1beta/files/abc123", true},
		{"https://us-central1-aiplatform.googleapis.com/v1/files/xyz", true},
		{"https://cdn.example.com/a.png", false},
		{"http://127.0.0.1/img.jpg", false},
		{"", false},
		{"data:image/png;base64,aaa", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, isGeminiNativeFileURI(tc.uri), tc.uri)
	}
}

func TestResolveGeminiImageURL_KeepsNativeFileData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	part := &dto.GeminiPart{
		FileData: &dto.GeminiFileData{
			FileUri: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		},
	}
	require.NoError(t, resolveGeminiImageURL(c, part))
	require.NotNil(t, part.FileData)
	assert.Nil(t, part.InlineData)
	assert.Equal(t, "video/webm", part.FileData.MimeType)
}

func TestResolveGeminiImageURL_HTTPFileDataToInlineData(t *testing.T) {
	withSSRFDisabled(t)
	gin.SetMode(gin.TestMode)

	// 1x1 PNG
	png := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41,
		0x54, 0x08, 0xd7, 0x63, 0xf8, 0xff, 0xff, 0x3f,
		0x00, 0x05, 0xfe, 0x02, 0xfe, 0xdc, 0xcc, 0x59,
		0xe7, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
		0x44, 0xae, 0x42, 0x60, 0x82,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	t.Cleanup(server.Close)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	part := &dto.GeminiPart{
		FileData: &dto.GeminiFileData{
			MimeType: "image/png",
			FileUri:  server.URL + "/a.png",
		},
	}
	require.NoError(t, resolveGeminiImageURL(c, part))
	require.NotNil(t, part.InlineData)
	assert.Nil(t, part.FileData)
	assert.Equal(t, "image/png", part.InlineData.MimeType)
	assert.NotEmpty(t, part.InlineData.Data)
	assert.Empty(t, part.InlineData.Url)
}

func TestResolveGeminiImageURL_InlineDataURLToData(t *testing.T) {
	withSSRFDisabled(t)
	gin.SetMode(gin.TestMode)

	png := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41,
		0x54, 0x08, 0xd7, 0x63, 0xf8, 0xff, 0xff, 0x3f,
		0x00, 0x05, 0xfe, 0x02, 0xfe, 0xdc, 0xcc, 0x59,
		0xe7, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
		0x44, 0xae, 0x42, 0x60, 0x82,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	t.Cleanup(server.Close)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	part := &dto.GeminiPart{
		InlineData: &dto.GeminiInlineData{
			MimeType: "image/png",
			Url:      server.URL + "/s3.png",
		},
	}
	require.NoError(t, resolveGeminiImageURL(c, part))
	require.NotNil(t, part.InlineData)
	assert.Empty(t, part.InlineData.Url)
	assert.Equal(t, base64.StdEncoding.EncodeToString(png), part.InlineData.Data)
}

func TestResolveGeminiImageURL_StripsGatewayURLWhenDataPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	part := &dto.GeminiPart{
		InlineData: &dto.GeminiInlineData{
			MimeType: "image/png",
			Data:     "abc",
			Url:      "https://cdn.example.com/x.png",
		},
	}
	require.NoError(t, resolveGeminiImageURL(c, part))
	assert.Equal(t, "abc", part.InlineData.Data)
	assert.Empty(t, part.InlineData.Url)
}

func TestGeminiFileDataSnakeCaseUnmarshal(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"file_data":{"mime_type":"image/png","file_uri":"https://cdn.example.com/a.png"}}`)
	var part dto.GeminiPart
	require.NoError(t, part.UnmarshalJSON(raw))
	require.NotNil(t, part.FileData)
	assert.Equal(t, "image/png", part.FileData.MimeType)
	assert.Equal(t, "https://cdn.example.com/a.png", part.FileData.FileUri)
}
