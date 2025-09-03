package config

import "time"

func (c *Config) ShutdownTimeout() time.Duration {
	return 15 * time.Second
}
