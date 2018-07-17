package chat

import (
	"testing"

	"github.com/maddevsio/comedian/config"
	"github.com/maddevsio/comedian/model"
	"github.com/nlopes/slack"
	"github.com/stretchr/testify/assert"
)

type MessageEvent struct {
}

func TestIsStandup(t *testing.T) {
	testCases := []struct {
		title   string
		input   string
		confirm bool
	}{
		{"all key words", "Yesterday managed to get docker up and running, today will complete test #100, problems: I have multilang!", true},
		{"no key words", "i want to create a standup but totaly forgot the way i should write it!", false},
		{"key words yesterday", "Yesterday it was fucking awesome!", false},
		{"key words yesterday and today", "Вчера ломал сервер, сегодня будет охренеть много дел", false},
		{"all key words capitalized", "Yesterday: launched MySQL, Today: will scream and should, Problems: SHIT IS ALL OVER!", true},
		{"keywords with mistakes", "Yesday completed shit, dotay will fap like crazy, promlems: no problems!", false},
	}
	c, err := config.Get()
	assert.NoError(t, err)
	s, err := NewSlack(c)
	assert.NoError(t, err)
	s.myUsername = "comedian"
	assert.NoError(t, err)
	for _, tt := range testCases {
		_, ok := s.isStandup(tt.input)
		if ok != tt.confirm {
			t.Errorf("Test %s: \n input: %s,\n expected confirm: %v\n actual confirm: %v \n", tt.title, tt.input, tt.confirm, ok)
		}
	}
}

func TestHandleMessage(t *testing.T) {
	c, err := config.Get()
	assert.NoError(t, err)
	s, err := NewSlack(c)
	assert.NoError(t, err)

	su1, err := s.db.CreateStandupUser(model.StandupUser{
		SlackUserID: "userID1",
		SlackName:   "user1",
		ChannelID:   "123qwe",
		Channel:     "general",
	})
	assert.NoError(t, err)

	msg := &slack.MessageEvent{}
	msg.Text = "<@> some message"
	msg.Channel = su1.Channel
	msg.Username = su1.SlackName

	err = s.handleMessage(msg)
	assert.NoError(t, err)
	assert.NoError(t, s.db.DeleteStandupByUsername(su1.SlackName))
	assert.NoError(t, s.db.DeleteStandupUserByUsername(su1.SlackName, su1.ChannelID))

}

// func TestHandleEditMessage(t *testing.T) {
// 	c, err := config.Get()
// 	assert.NoError(t, err)
// 	s, err := NewSlack(c)
// 	assert.NoError(t, err)

// 	su1, err := s.db.CreateStandupUser(model.StandupUser{
// 		SlackUserID: "userID1",
// 		SlackName:   "user1",
// 		ChannelID:   "123qwe",
// 		Channel:     "channel1",
// 	})
// 	assert.NoError(t, err)

// 	msg := &slack.MessageEvent{}
// 	msg.SubMessage.Text = "This standup is edited"
// 	err = s.handleMessage(msg)
// 	assert.NoError(t, err)
// 	assert.NoError(t, s.db.DeleteStandupUserByUsername(su1.SlackName, su1.ChannelID))

// }

func TestSendMessage(t *testing.T) {
	c, err := config.Get()
	assert.NoError(t, err)
	s, err := NewSlack(c)
	assert.NoError(t, err)
	err = s.SendMessage(c.DirectManagerChannelID, "MSG to manager!")
	assert.NoError(t, err)
}

func TestSendUserMessage(t *testing.T) {
	c, err := config.Get()
	assert.NoError(t, err)
	s, err := NewSlack(c)
	assert.NoError(t, err)

	su1, err := s.db.CreateStandupUser(model.StandupUser{
		SlackUserID: "UBA5V5W9K",
		SlackName:   "user1",
		ChannelID:   "123qwe",
		Channel:     "channel1",
	})
	assert.NoError(t, err)
	assert.Equal(t, "user1", su1.SlackName)
	assert.NotEqual(t, "userID1", su1.SlackUserID)

	err = s.SendUserMessage("USLACKBOT", "MSG to User!")
	assert.NoError(t, err)

	assert.NoError(t, s.db.DeleteStandupUserByUsername(su1.SlackName, su1.ChannelID))

}

func TestGetAllUsersToDB(t *testing.T) {
	c, err := config.Get()
	assert.NoError(t, err)
	s, err := NewSlack(c)
	assert.NoError(t, err)

	usersInChan, err := s.db.ListAllStandupUsers()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(usersInChan))

	err = s.GetAllUsersToDB()
	usersInChan, err = s.db.ListAllStandupUsers()
	assert.NoError(t, err)

	assert.True(t, len(usersInChan) > 0)

	for _, user := range usersInChan {
		assert.NoError(t, s.db.DeleteStandupUserByUsername(user.SlackName, user.ChannelID))
	}

}
