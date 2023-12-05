package config

type HMAC struct {
	Activated bool   `yaml:"activated" env:"HMAC_ACTIVATED" env-default:"false"`
	SecretKey string `yaml:"hmac_secret" env:"HMAC_SECRET" env-default:""`
	Salt      string `yaml:"hmac_salt" env:"HMAC_SALT" env-default:""`
	Length    int    `yaml:"hmac_length" env:"HMAC_LENGTH" env-default:"32"`
}
