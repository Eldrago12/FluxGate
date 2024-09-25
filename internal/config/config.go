package config

import (
	"fmt"
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

type Config struct {
	GCPProjectID string  `yaml:"gcp_project_id"`
	GCPRegion    string  `yaml:"gcp_region"`
	RedisName    string  `yaml:"redis_name"`
	ListenAddr   string  `yaml:"listen_addr"`
	Rate         float64 `yaml:"rate"`
	BucketSize   float64 `yaml:"bucket_size"`
}

func Load(filename string) (*Config, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	return &config, nil
}
