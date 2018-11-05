package chat

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/jasonlvhit/gocron"
	"github.com/maddevsio/comedian/config"
	"github.com/maddevsio/comedian/model"
	"github.com/maddevsio/comedian/storage"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/nlopes/slack"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"

	"strings"
)

var (
	typeMessage       = ""
	typeEditMessage   = "message_changed"
	typeDeleteMessage = "message_deleted"
)

// Slack struct used for storing and communicating with slack api
type Slack struct {
	API       *slack.Client
	RTM       *slack.RTM
	WG        sync.WaitGroup
	DB        *storage.MySQL
	Conf      config.Config
	Localizer *i18n.Localizer
}

// NewSlack creates a new copy of slack handler
func NewSlack(conf config.Config) (*Slack, error) {
	bundle := &i18n.Bundle{DefaultLanguage: language.English}
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	bundle.MustLoadMessageFile("active.ru.toml")
	localizer := i18n.NewLocalizer(bundle, conf.Language)
	db, err := storage.NewMySQL(conf)
	if err != nil {
		logrus.Errorf("slack: NewMySQL failed: %v\n", err)
		return nil, err
	}

	s := &Slack{}
	s.Conf = conf
	s.API = slack.New(conf.SlackToken)
	s.RTM = s.API.NewRTM()
	s.DB = db
	s.Localizer = localizer
	return s, nil
}

// Run runs a listener loop for slack
func (s *Slack) Run() {

	s.UpdateUsersList()
	helloManager := s.Localizer.MustLocalize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID:          "HelloManager",
			Description: "Greeting message once bot starts working",
			Other:       "Hello <@{{.ID}}>",
		},
		TemplateData: map[string]string{
			"ID": s.Conf.ManagerSlackUserID,
		},
	})
	s.SendUserMessage(s.Conf.ManagerSlackUserID, helloManager)

	gocron.Every(1).Day().At("23:50").Do(s.FillStandupsForNonReporters)
	gocron.Every(1).Day().At("23:55").Do(s.UpdateUsersList)
	gocron.Start()

	s.WG.Add(1)
	go s.RTM.ManageConnection()
	s.WG.Done()

	for msg := range s.RTM.IncomingEvents {
		switch ev := msg.Data.(type) {
		case *slack.MessageEvent:
			botUserID := fmt.Sprintf("<@%s>", s.RTM.GetInfo().User.ID)
			s.handleMessage(ev, botUserID)
		case *slack.MemberJoinedChannelEvent:
			s.handleJoin(ev.Channel)
		case *slack.InvalidAuthEvent:
			return
		}
	}
}

func (s *Slack) handleJoin(channelID string) {
	_, err := s.DB.SelectChannel(channelID)
	if err != nil {
		logrus.Error("No such channel found! Will create one!")
		channel, err := s.API.GetConversationInfo(channelID, true)
		if err != nil {
			logrus.Errorf("GetConversationInfo failed: %v", err)
		}
		createdChannel, err := s.DB.CreateChannel(model.Channel{
			ChannelName: channel.Name,
			ChannelID:   channel.ID,
			StandupTime: int64(0),
		})
		if err != nil {
			logrus.Errorf("CreateChannel failed: %v", err)
			return
		}
		logrus.Infof("New Channel Created: %v", createdChannel)
	}
}

func (s *Slack) handleMessage(msg *slack.MessageEvent, botUserID string) {
	standupCreated := s.Localizer.MustLocalize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID:    "StandupCreated",
			Other: "<@{{.ID}}>, your standup saved! Well done!",
		},
		TemplateData: map[string]string{
			"ID": msg.User,
		},
	})

	oneStandupPerDay := s.Localizer.MustLocalize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID:          "OneStandupPerDay",
			Description: "Warning that only one standup per day is allowed",
			Other:       "<@{{.ID}}>, you can submit only one standup per day. Please, edit today's standup or submit your next standup tomorrow!",
		},
		TemplateData: map[string]string{
			"ID": msg.User,
		},
	})

	couldNotSaveStandup := s.Localizer.MustLocalize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID:    "CouldNotSaveStandup",
			Other: "<@{{.ID}}>, something went wrong and I could not save your standup in database. Please, report this to your PM.",
		},
		TemplateData: map[string]string{
			"ID": msg.User,
		},
	})

	switch msg.SubType {
	case typeMessage:
		if !strings.Contains(msg.Msg.Text, botUserID) && !strings.Contains(msg.Msg.Text, "#standup") {
			return
		}
		messageIsStandup, problem := s.analizeStandup(msg.Msg.Text)
		if problem != "" {
			s.SendEphemeralMessage(msg.Channel, msg.User, problem)
			return
		}
		if messageIsStandup {
			if s.DB.SubmittedStandupToday(msg.User, msg.Channel) {

				s.SendEphemeralMessage(msg.Channel, msg.User, oneStandupPerDay)
				return
			}
			standup, err := s.DB.CreateStandup(model.Standup{
				ChannelID: msg.Channel,
				UserID:    msg.User,
				Comment:   msg.Msg.Text,
				MessageTS: msg.Msg.Timestamp,
			})
			if err != nil {
				logrus.Errorf("CreateStandup failed: %v", err)
				errorReportToManager := fmt.Sprintf("I could not save standup for user %s in channel %s because of the following reasons: %v", msg.User, msg.Channel, err)
				s.SendUserMessage(s.Conf.ManagerSlackUserID, errorReportToManager)
				s.SendEphemeralMessage(msg.Channel, msg.User, couldNotSaveStandup)
				return
			}
			logrus.Infof("Standup created #id:%v\n", standup.ID)
			item := slack.ItemRef{msg.Channel, msg.Msg.Timestamp, "", ""}
			time.Sleep(2 * time.Second)
			s.API.AddReaction("heavy_check_mark", item)
			s.SendEphemeralMessage(msg.Channel, msg.User, standupCreated)
			return
		}
	case typeEditMessage:
		if !strings.Contains(msg.SubMessage.Text, botUserID) && !strings.Contains(msg.SubMessage.Text, "#standup") {
			return
		}
		standup, err := s.DB.SelectStandupByMessageTS(msg.SubMessage.Timestamp)
		if err != nil {
			messageIsStandup, problem := s.analizeStandup(msg.SubMessage.Text)
			if problem != "" {
				s.SendEphemeralMessage(msg.Channel, msg.SubMessage.User, problem)
				return
			}
			if messageIsStandup {
				if s.DB.SubmittedStandupToday(msg.SubMessage.User, msg.Channel) {
					s.SendEphemeralMessage(msg.Channel, msg.SubMessage.User, oneStandupPerDay)
					return
				}
				logrus.Infof("CreateStandup while updating text ChannelID (%v), UserID (%v), Comment (%v), TimeStamp (%v)", msg.Channel, msg.SubMessage.User, msg.SubMessage.Text, msg.SubMessage.Timestamp)
				standup, err := s.DB.CreateStandup(model.Standup{
					ChannelID: msg.Channel,
					UserID:    msg.SubMessage.User,
					Comment:   msg.SubMessage.Text,
					MessageTS: msg.SubMessage.Timestamp,
				})
				if err != nil {
					logrus.Errorf("CreateStandup while updating text failed: %v", err)
					errorReportToManager := fmt.Sprintf("I could not create standup while updating msg for user %s in channel %s because of the following reasons: %v", msg.SubMessage.User, msg.Channel, err)
					s.SendUserMessage(s.Conf.ManagerSlackUserID, errorReportToManager)
					s.SendEphemeralMessage(msg.Channel, msg.SubMessage.User, couldNotSaveStandup)
					return
				}
				logrus.Infof("Standup created #id:%v\n", standup.ID)
				item := slack.ItemRef{msg.Channel, msg.SubMessage.Timestamp, "", ""}
				time.Sleep(2 * time.Second)
				s.API.AddReaction("heavy_check_mark", item)
				s.SendEphemeralMessage(msg.Channel, msg.SubMessage.User, standupCreated)
				return
			}
		}

		messageIsStandup, problem := s.analizeStandup(msg.SubMessage.Text)
		if problem != "" {
			s.SendEphemeralMessage(msg.Channel, msg.SubMessage.User, problem)
			return
		}
		if messageIsStandup {
			standup.Comment = msg.SubMessage.Text
			st, _ := s.DB.UpdateStandup(standup)
			logrus.Infof("Standup updated #id:%v\n", st.ID)
			time.Sleep(2 * time.Second)
			standupUpdated := s.Localizer.MustLocalize(&i18n.LocalizeConfig{
				DefaultMessage: &i18n.Message{
					ID:    "StandupUpdated",
					Other: "<@{{.ID}}>, your standup updated! Thanks!",
				},
				TemplateData: map[string]string{
					"ID": msg.User,
				},
			})
			s.SendEphemeralMessage(msg.Channel, msg.SubMessage.User, standupUpdated)
			return
		}

	case typeDeleteMessage:
		standup, err := s.DB.SelectStandupByMessageTS(msg.DeletedTimestamp)
		if err != nil {
			logrus.Errorf("SelectStandupByMessageTS failed: %v", err)
		}
		s.DB.DeleteStandup(standup.ID)
		logrus.Infof("Standup deleted #id:%v\n", standup.ID)
	}
}

func (s *Slack) analizeStandup(message string) (bool, string) {
	noProblems := s.Localizer.MustLocalize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID:    "NoProblemKeysDetected",
			Other: "No 'problems' related keywords detected! Please, use one of the following: 'problem', 'difficult', 'stuck', 'question', 'issue'",
		},
	})

	noYesterdayWork := s.Localizer.MustLocalize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID:    "NoYesterdayWorkKeysDetected",
			Other: "No 'yesterday' related keywords detected! Please, use one of the following: 'yesterday', 'friday', 'completed'",
		},
	})
	noTodayPlans := s.Localizer.MustLocalize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID:    "NoTodayPlansKeysDetected",
			Other: "No 'today' related keywords detected! Please, use one of the following: 'today', 'going', 'plan'",
		},
	})
	message = strings.ToLower(message)
	mentionsProblem := false
	problemKeys := []string{"problem", "difficult", "stuck", "question", "issue", "block", "проблем", "трудност", "затрдуднени", "вопрос"}
	for _, problem := range problemKeys {
		if strings.Contains(message, problem) {
			mentionsProblem = true
		}
	}
	if !mentionsProblem {
		return false, noProblems
	}

	mentionsYesterdayWork := false
	yesterdayWorkKeys := []string{"yesterday", "friday", "completed", "вчера", "пятниц", "делал", "сделано"}
	for _, work := range yesterdayWorkKeys {
		if strings.Contains(message, work) {
			mentionsYesterdayWork = true
		}
	}
	if !mentionsYesterdayWork {
		return false, noYesterdayWork
	}

	mentionsTodayPlans := false
	todayPlansKeys := []string{"today", "going", "plan", "сегодня", "собираюсь", "план"}
	for _, plan := range todayPlansKeys {
		if strings.Contains(message, plan) {
			mentionsTodayPlans = true
		}
	}
	if !mentionsTodayPlans {
		return false, noTodayPlans
	}
	return true, ""
}

// SendMessage posts a message in a specified channel visible for everyone
func (s *Slack) SendMessage(channel, message string, attachments []slack.Attachment) error {
	_, _, err := s.API.PostMessage(channel, message, slack.PostMessageParameters{
		Attachments: attachments,
	})
	if err != nil {
		logrus.Errorf("slack: PostMessage failed: %v\n", err)
		return err
	}
	return err
}

// SendEphemeralMessage posts a message in a specified channel which is visible only for selected user
func (s *Slack) SendEphemeralMessage(channel, user, message string) error {
	_, err := s.API.PostEphemeral(
		channel,
		user,
		slack.MsgOptionText(message, true),
	)
	if err != nil {
		logrus.Errorf("slack: PostEphemeral failed: %v\n", err)
		return err
	}
	return err
}

// SendUserMessage Direct Message specific user
func (s *Slack) SendUserMessage(userID, message string) error {
	_, _, channelID, err := s.API.OpenIMChannel(userID)
	if err != nil {
		return err
	}
	err = s.SendMessage(channelID, message, nil)
	if err != nil {
		return err
	}
	return err
}

//UpdateUsersList updates users in workspace
func (s *Slack) UpdateUsersList() {
	users, err := s.API.GetUsers()
	if err != nil {
		logrus.Errorf("GetUsers failed: %v", err)
		return
	}
	for _, user := range users {
		if user.IsBot || user.Name == "slackbot" {
			continue
		}

		u, err := s.DB.SelectUser(user.ID)
		if err != nil {
			if user.IsAdmin || user.IsOwner || user.IsPrimaryOwner {
				s.DB.CreateUser(model.User{
					UserName: user.Name,
					UserID:   user.ID,
					Role:     "admin",
				})
				continue
			}
			s.DB.CreateUser(model.User{
				UserName: user.Name,
				UserID:   user.ID,
				Role:     "",
			})
		}
		if user.Deleted {
			s.DB.DeleteUser(u.ID)
			cm, err := s.DB.FindMembersByUserID(u.UserID)
			if err != nil {
				continue
			}
			for _, member := range cm {
				s.DB.DeleteChannelMember(member.UserID, member.ChannelID)
				tt, err := s.DB.SelectTimeTable(member.ID)
				if err != nil {
					continue
				}
				s.DB.DeleteTimeTable(tt.ID)
			}
		}
	}
	logrus.Info("Users list updated successfully")
}

//FillStandupsForNonReporters fills standup entries with empty standups to later recognize
//non reporters vs those who did not have to write standups
func (s *Slack) FillStandupsForNonReporters() {
	if int(time.Now().Weekday()) == 6 || int(time.Now().Weekday()) == 0 {
		return
	}
	allUsers, err := s.DB.ListAllChannelMembers()
	if err != nil {
		return
	}
	for _, user := range allUsers {
		if user.Created.Day() == time.Now().Day() {
			continue
		}
		hasStandup := s.DB.SubmittedStandupToday(user.UserID, user.ChannelID)
		if !hasStandup {
			_, err := s.DB.CreateStandup(model.Standup{
				ChannelID: user.ChannelID,
				UserID:    user.UserID,
				Comment:   "",
				MessageTS: strconv.Itoa(int(time.Now().Unix())),
			})
			if err != nil {
				errorReportToManager := fmt.Sprintf("I could not create empty standup for user %s in channel %s because of the following reasons: %v", user.UserID, user.ChannelID, err)
				s.SendUserMessage(s.Conf.ManagerSlackUserID, errorReportToManager)
			}
		}
	}
}
