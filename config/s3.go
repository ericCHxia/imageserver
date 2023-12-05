package config

type S3 struct {
	AccessKey string `yaml:"s3_access_key" env:"S3_ACCESS_KEY" env-default:""`
	SecretKey string `yaml:"s3_secret_key" env:"S3_SECRET_KEY" env-default:""`
	Region    string `yaml:"s3_region" env:"S3_REGION" env-default:""`
	EndPoint  string `yaml:"s3_endpoint" env:"S3_ENDPOINT" env-default:""`
	Bucket    string `yaml:"s3_bucket" env:"S3_BUCKET" env-default:""`
	CacheTime int64  `yaml:"s3_cache_time" env:"S3_CACHE_TIME" env-default:"-1"`
	FleshTime int64  `yaml:"s3_flesh_time" env:"S3_FLESH_TIME" env-default:"1800"`
}
