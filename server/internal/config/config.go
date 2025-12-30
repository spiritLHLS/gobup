package config

import "sync"

type Config struct {
	Port     int
	WorkPath string
	DataPath string
	// 仅用于初始化管理员账号
	InitUsername string
	InitPassword string
}

var (
	AppConfig = &Config{}
	once      sync.Once
)

func Init(port int, workPath, username, password, dataPath string) {
	once.Do(func() {
		AppConfig.Port = port
		AppConfig.WorkPath = workPath
		AppConfig.InitUsername = username
		AppConfig.InitPassword = password
		AppConfig.DataPath = dataPath
	})
}
