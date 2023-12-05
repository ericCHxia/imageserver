package config

type Config struct {
	Hmac            HMAC            `yaml:"hmac"`
	S3              S3              `yaml:"s3"`
	CacheController CacheController `yaml:"cache_controller"`
	Cache           Cache           `yaml:"cache"`
	Base64Path      bool            `yaml:"base64_path" env:"BASE64_PATH" env-default:"false"`
}
