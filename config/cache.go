package config

type CacheController struct {
	Activated bool `yaml:"activated" env:"CACHE_CONTROLLER_ACTIVATED" env-default:"false"`
	MaxAge    int  `yaml:"max_age" env:"CACHE_CONTROLLER_MAX_AGE" env-default:"3600"`
}

type Cache struct {
	Activated bool  `yaml:"activated" env:"CACHE_ACTIVATED" env-default:"false"`
	Time      int64 `yaml:"time" env:"CACHE_TIME" env-default:"3600"`
}
