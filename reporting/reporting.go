package reporting

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/maddevsio/comedian/config"
	"github.com/maddevsio/comedian/model"
	"github.com/maddevsio/comedian/storage"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/sirupsen/logrus"
)

type reportContent struct {
	Channel      string
	Standups     []model.Standup
	NonReporters []model.StandupUser
}

type reportEntry struct {
	DateFrom       time.Time
	DateTo         time.Time
	ReportContents []reportContent
}

// UserData used to parse data on user from Collector
type UserData struct {
	TotalCommits int `json:"total_commits"`
	TotalMerges  int `json:"total_merges"`
	Worklogs     int `json:"worklogs"`
}

// ProjectData used to parse data on project from Collector
type ProjectData struct {
	TotalCommits int `json:"total_commits"`
	TotalMerges  int `json:"total_merges"`
}

// ProjectUserData used to parse data on user in project from Collector
type ProjectUserData struct {
	TotalCommits int `json:"total_commits"`
	TotalMerges  int `json:"total_merges"`
}

var localizer *i18n.Localizer

func initLocalizer() *i18n.Localizer {
	localizer, err := config.GetLocalizer()
	if err != nil {
		logrus.Errorf("reporting: GetLocalizer failed: %v\n", err)
		return nil
	}
	return localizer
}

// StandupReportByProject creates a standup report for a specified period of time
func StandupReportByProject(db storage.Storage, channelID string, dateFrom, dateTo time.Time, collectorData []byte) (string, error) {
	localizer = initLocalizer()
	channel := strings.Replace(channelID, "#", "", -1)
	reportEntries, err := getReportEntriesForPeriodByChannel(db, channel, dateFrom, dateTo)
	if err != nil {
		logrus.Errorf("reporting: getReportEntriesForPeriodByChannel failed: %v\n", err)
		return "Error!", err
	}
	logrus.Infof("report entries: %#v\n", reportEntries)
	text, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "reportOnProjectHead"})
	if err != nil {
		logrus.Errorf("reporting: Localize failed: %v\n", err)
	}
	report := fmt.Sprintf(text, channel)
	report += ReportEntriesForPeriodByChannelToString(reportEntries)

	var dataP ProjectData
	json.Unmarshal(collectorData, &dataP)

	report += fmt.Sprintf("\n\nCommits for period: %v \nMerges for period: %v\n", dataP.TotalCommits, dataP.TotalMerges)
	return report, nil
}

// StandupReportByUser creates a standup report for a specified period of time
func StandupReportByUser(db storage.Storage, user model.StandupUser, dateFrom, dateTo time.Time, collectorData []byte) (string, error) {
	localizer = initLocalizer()
	reportEntries, err := getReportEntriesForPeriodbyUser(db, user, dateFrom, dateTo)
	if err != nil {
		logrus.Errorf("reporting: getReportEntriesForPeriodbyUser failed: %v\n", err)
		return "Error!", err
	}
	logrus.Infof("reporting: report entries: %#v\n", reportEntries)
	text, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "reportOnUserHead"})
	if err != nil {
		logrus.Errorf("reporting: Localize failed: %v\n", err)
	}
	report := fmt.Sprintf(text, user.SlackName)
	report += ReportEntriesByUserToString(reportEntries)

	var dataU UserData
	json.Unmarshal(collectorData, &dataU)

	report += fmt.Sprintf("\n\nCommits for period: %v \nMerges for period: %v\nWorklogs: %v hours", dataU.TotalCommits, dataU.TotalMerges, dataU.Worklogs/3600)

	return report, nil
}

// StandupReportByProjectAndUser creates a standup report for a specified period of time
func StandupReportByProjectAndUser(db storage.Storage, channelID string, user model.StandupUser, dateFrom, dateTo time.Time, collectorData []byte) (string, error) {
	localizer = initLocalizer()
	channel := strings.Replace(channelID, "#", "", -1)
	reportEntries, err := getReportEntriesForPeriodByChannelAndUser(db, channel, user, dateFrom, dateTo)
	if err != nil {
		logrus.Errorf("reporting: getReportEntriesForPeriodByChannelAndUser: %v\n", err)
		return "Error!", err
	}
	logrus.Infof("reporting: report entries: %#v\n", reportEntries)

	text, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "reportOnProjectAndUserHead"})
	if err != nil {
		logrus.Errorf("reporting: Localize failed: %v\n", err)

	}
	report := fmt.Sprintf(text, channel, user.SlackName)
	report += ReportEntriesForPeriodByChannelToString(reportEntries)

	var dataPU ProjectUserData
	json.Unmarshal(collectorData, &dataPU)

	report += fmt.Sprintf("\n\nCommits for period: %v \nMerges for period: %v\n", dataPU.TotalCommits, dataPU.TotalMerges)

	return report, nil
}

//getReportEntriesForPeriodByChannel returns report entries by channel
func getReportEntriesForPeriodByChannel(db storage.Storage, channelID string, dateFrom, dateTo time.Time) ([]reportEntry, error) {
	dateFromRounded, numberOfDays, err := setupDays(dateFrom, dateTo)
	if err != nil {
		logrus.Errorf("reporting: setupDays failed: %v\n", err)
		return nil, err
	}
	logrus.Infof("reporting: chanReport, channel: <#%v>", channelID)

	reportEntries := make([]reportEntry, 0, numberOfDays)
	for day := 0; day <= numberOfDays; day++ {
		currentDateFrom := dateFromRounded.Add(time.Duration(day*24) * time.Hour)
		currentDateTo := currentDateFrom.Add(24 * time.Hour)

		currentDayStandups, err := db.SelectStandupsByChannelIDForPeriod(channelID, currentDateFrom, currentDateTo)
		if err != nil {
			logrus.Errorf("reporting: SelectStandupsByChannelIDForPeriod failed: %v", err)
			return nil, err
		}
		currentDayNonReporters, err := db.GetNonReporters(channelID, currentDateFrom, currentDateTo)
		if err != nil {
			logrus.Errorf("reporting: SelectStandupsByChannelIDForPeriod failed: %v", err)
			return nil, err
		}
		logrus.Infof("reporting: chanReport, current day standups: %v\n", currentDayStandups)
		logrus.Infof("reporting: chanReport, current day non reporters: %v\n", currentDayNonReporters)
		if len(currentDayNonReporters) > 0 || len(currentDayStandups) > 0 {
			reportContents := make([]reportContent, 0, 1)
			reportContents = append(reportContents,
				reportContent{
					Standups:     currentDayStandups,
					NonReporters: currentDayNonReporters})

			reportEntries = append(reportEntries,
				reportEntry{
					DateFrom:       currentDateFrom,
					DateTo:         currentDateTo,
					ReportContents: reportContents})
		}
	}
	return reportEntries, nil
}

//ReportEntriesForPeriodByChannelToString returns report entries by channel in text
func ReportEntriesForPeriodByChannelToString(reportEntries []reportEntry) string {
	localizer = initLocalizer()
	var report string
	if len(reportEntries) == 0 {
		text, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "reportNoData"})
		if err != nil {
			logrus.Errorf("reporting: Localize failed: %v\n", err)

		}
		return report + text
	}

	for _, value := range reportEntries {
		currentDateFrom := value.DateFrom
		currentDateTo := value.DateTo
		text, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "reportPeriod"})
		if err != nil {
			logrus.Errorf("reporting: Localize failed: %v\n", err)

		}
		report += fmt.Sprintf(text, currentDateFrom.Format("2006-01-02"),
			currentDateTo.Format("2006-01-02"))
		for _, standup := range value.ReportContents[0].Standups {
			text, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "reportStandupFromUser"})
			if err != nil {
				logrus.Errorf("reporting: Localize failed: %v\n", err)

			}
			report += fmt.Sprintf(text, standup.Username, standup.Comment)
		}
		for _, user := range value.ReportContents[0].NonReporters {
			text, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "reportIgnoredStandup"})
			if err != nil {
				logrus.Errorf("reporting: Localize failed: %v\n", err)

			}
			report += fmt.Sprintf(text, user.SlackName)
		}
	}

	return report
}

func getReportEntriesForPeriodbyUser(db storage.Storage, user model.StandupUser, dateFrom, dateTo time.Time) ([]reportEntry, error) {
	dateFromRounded, numberOfDays, err := setupDays(dateFrom, dateTo)
	if err != nil {
		logrus.Errorf("reporting: setupDays failed: %v\n", err)
		return nil, err
	}
	reportEntries := make([]reportEntry, 0, numberOfDays)
	for day := 0; day <= numberOfDays; day++ {
		currentDateFrom := dateFromRounded.Add(time.Duration(day*24) * time.Hour)
		currentDateTo := currentDateFrom.Add(24 * time.Hour)

		standupUsers, err := db.FindStandupUsers(user.SlackName)
		if err != nil {
			logrus.Errorf("reporting: FindStandupUser failed: %v\n", err)
			return nil, err
		}
		reportContents := make([]reportContent, 0, len(standupUsers))
		for _, standupUser := range standupUsers {
			currentDayNonReporter := []model.StandupUser{}
			currentDayStandup, err := db.SelectStandupsFiltered(standupUser.SlackUserID, standupUser.ChannelID, currentDateFrom, currentDateTo)

			currentDayNonReporter, err = db.GetNonReporter(user.SlackUserID, standupUser.ChannelID, currentDateFrom, currentDateTo)
			if err != nil {
				logrus.Errorf("reporting: SelectStandupsByChannelIDForPeriod failed: %v", err)
				return nil, err
			}

			logrus.Infof("reporting: userReport, current day standups: %#v\n", currentDayStandup)
			logrus.Infof("reporting: userReport, current day non reporters: %#v\n", currentDayNonReporter)

			if len(currentDayNonReporter) > 0 || len(currentDayStandup) > 0 {
				reportContents = append(reportContents,
					reportContent{
						Channel:      standupUser.ChannelID,
						Standups:     currentDayStandup,
						NonReporters: currentDayNonReporter})
			}

		}
		reportEntries = append(reportEntries,
			reportEntry{
				DateFrom:       currentDateFrom,
				DateTo:         currentDateTo,
				ReportContents: reportContents})
	}
	logrus.Infof("reporting: userReport, final report entries: %#v\n", reportEntries)
	return reportEntries, nil
}

//ReportEntriesByUserToString provides reporting entries in selected time period
func ReportEntriesByUserToString(reportEntries []reportEntry) string {
	localizer = initLocalizer()
	var report string
	emptyReport := true
	for _, reportEntry := range reportEntries {
		if len(reportEntry.ReportContents) != 0 {
			emptyReport = false
		}
	}
	if emptyReport {
		text, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "reportNoData"})
		if err != nil {
			logrus.Errorf("reporting: Localize failed: %v\n", err)

		}
		return report + text
	}

	for _, value := range reportEntries {
		if len(value.ReportContents) == 0 {
			continue
		}
		currentDateFrom := value.DateFrom
		currentDateTo := value.DateTo

		text, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "reportPeriod"})
		if err != nil {
			logrus.Errorf("reporting: Localize failed: %v\n", err)

		}
		report += fmt.Sprintf(text, currentDateFrom.Format("2006-01-02"),
			currentDateTo.Format("2006-01-02"))
		for _, reportContent := range value.ReportContents {
			text, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "reportShowChannel"})
			if err != nil {
				logrus.Errorf("reporting: Localize failed: %v\n", err)
			}
			report += fmt.Sprintf(text, reportContent.Channel)
			for _, standup := range reportContent.Standups {
				report += standup.Comment + "\n"
			}
			for _, user := range reportContent.NonReporters {
				text, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "reportIgnoredStandup"})
				if err != nil {
					logrus.Errorf("reporting: Localize failed: %v\n", err)

				}
				report += fmt.Sprintf(text, user.SlackName)
			}
			report += "\n"

		}

	}
	return report
}

//getReportEntriesForPeriodByChannelAndUser returns report entries by channel
func getReportEntriesForPeriodByChannelAndUser(db storage.Storage, channelID string, user model.StandupUser, dateFrom, dateTo time.Time) ([]reportEntry, error) {
	dateFromRounded, numberOfDays, err := setupDays(dateFrom, dateTo)
	if err != nil {
		logrus.Errorf("reporting: setupDays failed: %v\n", err)
		return nil, err
	}

	reportEntries := make([]reportEntry, 0, numberOfDays)
	for day := 0; day <= numberOfDays; day++ {
		currentDateFrom := dateFromRounded.Add(time.Duration(day*24) * time.Hour)
		currentDateTo := currentDateFrom.Add(24 * time.Hour)

		currentDayStandup, err := db.SelectStandupsFiltered(user.SlackUserID, channelID, currentDateFrom, currentDateTo)
		if err != nil {
			logrus.Errorf("reporting: SelectStandups failed: %v\n", err)
		}

		currentDayNonReporter, err := db.GetNonReporter(user.SlackUserID, channelID, currentDateFrom, currentDateTo)
		if err != nil {
			logrus.Errorf("reporting: SelectStandupsByChannelIDForPeriod failed: %v", err)
			return nil, err
		}

		logrus.Infof("reporting: projectUserReport, current day standups: %#v\n", currentDayStandup)
		logrus.Infof("reporting: projectUserReport, current day non reporters: %#v\n", currentDayNonReporter)

		if len(currentDayNonReporter) > 0 || len(currentDayStandup) > 0 {
			reportContents := make([]reportContent, 0, 1)
			reportContents = append(reportContents,
				reportContent{
					Standups:     currentDayStandup,
					NonReporters: currentDayNonReporter})

			reportEntries = append(reportEntries,
				reportEntry{
					DateFrom:       currentDateFrom,
					DateTo:         currentDateTo,
					ReportContents: reportContents})
		}

	}
	logrus.Infof("reporting: projectUserReport, final report entries: %#v\n", reportEntries)
	return reportEntries, nil
}

//setupDays gets dates and returns their differense in days
func setupDays(dateFrom, dateTo time.Time) (time.Time, int, error) {
	if dateTo.Before(dateFrom) {
		err := errors.New("Starting date is bigger than end date")
		logrus.Errorf("reporting: setupDays Before failed: %v\n", err)
		return time.Now(), 0, err
	}
	if dateTo.After(time.Now()) {
		err := errors.New("Report end time was in the future, time range was truncated")
		logrus.Errorf("reporting: setupDays After failed: %v\n", err)
		return time.Now(), 0, err
	}

	dateFromRounded := time.Date(dateFrom.Year(), dateFrom.Month(), dateFrom.Day(), 0, 0, 0, 0, time.UTC)
	dateToRounded := time.Date(dateTo.Year(), dateTo.Month(), dateTo.Day(), 0, 0, 0, 0, time.UTC)
	dateDiff := dateToRounded.Sub(dateFromRounded)
	numberOfDays := int(dateDiff.Hours() / 24)
	return dateFromRounded, numberOfDays, nil
}
