package config

import "sync"

type Config struct {
	Port        int
	WorkPath    string
	Username    string
	Password    string
	DataPath    string
	WxPushToken string // WxPusher token
}

var (
	AppConfig = &Config{}
	once      sync.Once
)

func Init(port int, workPath, username, password, dataPath, wxPushToken string) {
	once.Do(func() {
		AppConfig.Port = port
		AppConfig.WorkPath = workPath
		AppConfig.Username = username
		AppConfig.Password = password
		AppConfig.DataPath = dataPath
		AppConfig.WxPushToken = wxPushToken
	})
}
