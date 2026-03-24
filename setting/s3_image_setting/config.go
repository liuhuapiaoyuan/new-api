package s3_image_setting

import "github.com/QuantumNous/new-api/setting/config"

// S3ImageSetting 控制台 S3 图片存储（与 env S3_* 对应；secret 对应 S3_SECRET_ACCESS_KEY）
type S3ImageSetting struct {
	Enabled      bool   `json:"enabled"`
	Bucket       string `json:"bucket"`
	Region       string `json:"region"`
	Endpoint     string `json:"endpoint"`
	AccessKeyId  string `json:"access_key_id"`
	Secret       string `json:"secret"`
	Cdn          string `json:"cdn"`
	// Dir 可选对象键前缀（子目录），与请求 S3_DIR / 环境变量 S3_DIR 合并时请求优先。
	Dir string `json:"dir"`
}

var defaultS3ImageSetting = S3ImageSetting{}

func init() {
	config.GlobalConfig.Register("s3_image_setting", &defaultS3ImageSetting)
}

// GetS3ImageSetting 返回全局 S3 图片配置指针
func GetS3ImageSetting() *S3ImageSetting {
	return &defaultS3ImageSetting
}
