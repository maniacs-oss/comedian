package config

import (
	"github.com/BurntSushi/toml"
	"github.com/kelseyhightower/envconfig"
	"github.com/labstack/gommon/log"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	logrus "github.com/sirupsen/logrus"
	"golang.org/x/text/language"
)

type (
	// Config struct used for configuration of app with env variables
	Config struct {
		SlackToken             string `envconfig:"SLACK_TOKEN" required:"true"`
		DatabaseURL            string `envconfig:"DATABASE" required:"true"`
		HTTPBindAddr           string `envconfig:"HTTP_BIND_ADDR" required:"true"`
		NotifierCheckInterval  uint64 `envconfig:"NOTIFIER_CHECK_INTERVAL" required:"true"`
		Manager                string `envconfig:"MANAGER" required:"true"`
		DirectManagerChannelID string `envconfig:"DIRECT_MANAGER_CHANNEL_ID" required:"true"`
		ReportTime             string `envconfig:"REPORT_TIME" required:"true"`
		Language               string `envconfig:"LANGUAGE" required:"false"`
		Debug                  bool
	}
)

// Get method processes env variables and fills Config struct
func Get() (Config, error) {
	var c Config
	if err := envconfig.Process("comedian", &c); err != nil {
		log.Errorf("ERROR: %s", err.Error())
		return c, err
	}
	return c, nil
}

// GetLocalizer creates localizer instance that can be used by app packages to localize translations
func GetLocalizer() (*i18n.Localizer, error) {
	c, err := Get()
	if err != nil {
		return nil, err
	}
	bundle := &i18n.Bundle{DefaultLanguage: language.English}
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	_, err = bundle.LoadMessageFile("ru.toml")
	if err != nil {
		logrus.Error(err)
		return nil, err
	}
	_, err = bundle.LoadMessageFile("en.toml")
	if err != nil {
		logrus.Error(err)
		return nil, err
	}
	localizer := i18n.NewLocalizer(bundle, c.Language)
	return localizer, nil
}
