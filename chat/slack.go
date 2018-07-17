package chat

import (
	"sync"

	"github.com/maddevsio/comedian/config"
	"github.com/maddevsio/comedian/model"
	"github.com/maddevsio/comedian/storage"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/nlopes/slack"
	"github.com/sirupsen/logrus"

	"strings"
)

var (
	typeMessage     = ""
	typeEditMessage = "message_changed"
	localizer       *i18n.Localizer
)

// Slack struct used for storing and communicating with slack api
type Slack struct {
	Chat
	api        *slack.Client
	logger     *logrus.Logger
	rtm        *slack.RTM
	wg         sync.WaitGroup
	myUsername string
	db         *storage.MySQL
}

// NewSlack creates a new copy of slack handler
func NewSlack(conf config.Config) (*Slack, error) {
	m, err := storage.NewMySQL(conf)
	if err != nil {
		logrus.Errorf("ERROR: %s", err.Error())
		return nil, err
	}
	s := &Slack{}
	s.api = slack.New(conf.SlackToken)
	s.logger = logrus.New()
	s.rtm = s.api.NewRTM()
	s.db = m

	localizer, err = config.GetLocalizer()
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Run runs a listener loop for slack
func (s *Slack) Run() error {

	s.ManageConnection()
	for {
		if s.myUsername == "" {
			info := s.rtm.GetInfo()
			if info != nil {
				s.myUsername = info.User.ID
			}
		}
		select {
		case msg := <-s.rtm.IncomingEvents:

			switch ev := msg.Data.(type) {
			case *slack.ConnectedEvent:
				s.GetAllUsersToDB()
				text, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "HelloManager"})
				if err != nil {
					s.logger.Error(err)
				}
				s.SendUserMessage("UB9AE7CL9", text)

			case *slack.MessageEvent:
				s.handleMessage(ev)
			case *slack.PresenceChangeEvent:
				s.logger.Infof("Presence Change: %v\n", ev)

			case *slack.RTMError:
				logrus.Errorf("ERROR: %s", ev.Error())

			case *slack.InvalidAuthEvent:
				s.logger.Info("Invalid credentials")
				return nil
			}
		}
	}
}

// ManageConnection manages connection
func (s *Slack) ManageConnection() {
	s.wg.Add(1)
	go func() {
		s.rtm.ManageConnection()
		s.wg.Done()
	}()

}

func (s *Slack) handleMessage(msg *slack.MessageEvent) error {

	switch msg.SubType {
	case typeMessage:
		if standupText, ok := s.isStandup(msg.Msg.Text); ok {
			_, err := s.db.CreateStandup(model.Standup{
				Channel:    msg.Msg.Channel,
				UsernameID: msg.Msg.User,
				Username:   msg.Msg.Username,
				Comment:    standupText,
				MessageTS:  msg.Msg.Timestamp,
			})
			var text string
			if err != nil {
				logrus.Errorf("ERROR: %s", err.Error())
				return err
			}
			text, err = localizer.Localize(&i18n.LocalizeConfig{MessageID: "StandupAccepted"})
			if err != nil {
				s.logger.Error(err)
			}
			return s.SendMessage(msg.Msg.Channel, text)

		}
	case typeEditMessage:
		standup, err := s.db.SelectStandupByMessageTS(msg.SubMessage.Timestamp)
		if err != nil {
			logrus.Errorf("ERROR: %s", err.Error())
			return err
		}
		_, err = s.db.AddToStandupHistory(model.StandupEditHistory{
			StandupID:   standup.ID,
			StandupText: standup.Comment})
		if err != nil {
			logrus.Errorf("ERROR: %s", err.Error())
			return err
		}
		if standupText, ok := s.isStandup(msg.SubMessage.Text); ok {
			standup.Comment = standupText

			_, err = s.db.UpdateStandup(standup)
			if err != nil {
				logrus.Errorf("ERROR: %s", err.Error())
				return err
			}
			s.logger.Info("Edited")
		}
	}
	return nil
}

func (s *Slack) isStandup(message string) (string, bool) {

	p1, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "p1"})
	if err != nil {
		s.logger.Error(err)
	}
	p2, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "p2"})
	if err != nil {
		s.logger.Error(err)
	}
	p3, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "p3"})
	if err != nil {
		s.logger.Error(err)
	}

	mentionsProblem := false
	problemKeys := []string{p1, p2, p3}
	for _, problem := range problemKeys {
		if strings.Contains(message, problem) {
			mentionsProblem = true
		}
	}

	y1, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "y1"})
	if err != nil {
		s.logger.Error(err)
	}
	y2, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "y2"})
	if err != nil {
		s.logger.Error(err)
	}
	y3, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "y3"})
	if err != nil {
		s.logger.Error(err)
	}
	y4, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "y4"})
	if err != nil {
		s.logger.Error(err)
	}
	mentionsYesterdayWork := false
	yesterdayWorkKeys := []string{y1, y2, y3, y4}
	for _, work := range yesterdayWorkKeys {
		if strings.Contains(message, work) {
			mentionsYesterdayWork = true
		}
	}

	t1, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "t1"})
	if err != nil {
		s.logger.Error(err)
	}
	t2, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "t2"})
	if err != nil {
		s.logger.Error(err)
	}
	t3, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "t3"})
	if err != nil {
		s.logger.Error(err)
	}
	mentionsTodayPlans := false
	todayPlansKeys := []string{t1, t2, t3}
	for _, plan := range todayPlansKeys {
		if strings.Contains(message, plan) {
			mentionsTodayPlans = true
		}
	}

	if mentionsProblem && mentionsYesterdayWork && mentionsTodayPlans {
		logrus.Infof("This message is a standup: %v", message)
		return strings.TrimSpace(message), true
	}

	logrus.Errorf("This message is not a standup: %v", message)
	return message, false

}

// SendMessage posts a message in a specified channel
func (s *Slack) SendMessage(channel, message string) error {
	_, _, err := s.api.PostMessage(channel, message, slack.PostMessageParameters{})
	return err
}

// SendUserMessage posts a message to a specific user
func (s *Slack) SendUserMessage(userID, message string) error {
	_, _, channelID, err := s.api.OpenIMChannel(userID)
	logrus.Println(channelID)
	if err != nil {
		logrus.Errorf("ERROR: %s", err.Error())
		return err
	}
	err = s.SendMessage(channelID, message)
	return err
}

// GetAllUsersToDB selects all the users in the organization and sync them to db
func (s *Slack) GetAllUsersToDB() error {
	users, err := s.api.GetUsers()
	if err != nil {
		logrus.Errorf("ERROR: %s", err.Error())
		return err
	}
	chans, err := s.api.GetChannels(false)
	if err != nil {
		logrus.Errorf("ERROR: %s", err.Error())
		return err
	}
	var channelID string
	for _, channel := range chans {
		if channel.Name == "general" {
			channelID = channel.ID
		}
	}
	for _, user := range users {
		_, err := s.db.FindStandupUserInChannel(user.Name, channelID)
		if err != nil {
			s.db.CreateStandupUser(model.StandupUser{
				SlackUserID: user.ID,
				SlackName:   user.Name,
				ChannelID:   "",
				Channel:     "",
			})
		}

	}
	return nil
}
