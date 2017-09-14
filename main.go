package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/codegangsta/cli"
	"github.com/jhoonb/archivex"
	"github.com/nlopes/slack"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func main() {
	app := cli.NewApp()
	app.Name = "slack-dump"
	app.Usage = "export channel and group history to the Slack export format include Direct message"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "token, t",
			Value:  "",
			Usage:  "a Slack API token: (see: https://api.slack.com/web)",
			EnvVar: "SLACK_API_TOKEN",
		},
		cli.BoolFlag{
			Name:   "text, x",
			Usage:  "Output plain text instead of json files.",
		},
	}
	app.Author = "Joe Fitzgerald, Sunyong Lim"
	app.Email = "jfitzgerald@pivotal.io, dicebattle@gmail.com"
	app.Version = "0.0.2"
	app.Action = func(c *cli.Context) {
		token := c.String("token")
		if token == "" {
			fmt.Println("ERROR: the token flag is required...")
			fmt.Println("")
			cli.ShowAppHelp(c)
			os.Exit(2)
		}
		textOutput := c.Bool("text")
		roomsOrUsers := c.Args()
		api := slack.New(token)
		_, err := api.AuthTest()
		if err != nil {
			fmt.Println("ERROR: the token you used is not valid...")
			os.Exit(2)
		}

		// Create working directory
		dir, err := ioutil.TempDir("", "slack-dump")
		check(err)

		// Dump Users
		usersMap := dumpUsers(api, dir, roomsOrUsers, textOutput)

		// Dump Channels and Groups
		dumpRooms(api, dir, roomsOrUsers, usersMap, textOutput)

		archive(dir)
	}

	app.Run(os.Args)
}

func archive(dir string) {
	zip := new(archivex.ZipFile)
	pwd, err := os.Getwd()
	check(err)
	zip.Create(path.Join(pwd, "slackdump.zip"))
	zip.AddAll(dir, true)
	zip.Close()
}

// MarshalIndent is like json.MarshalIndent but applies Slack's weird JSON
// escaping rules to the output.
func MarshalIndent(v interface{}, prefix string, indent string) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "    ")
	if err != nil {
		return nil, err
	}

	b = bytes.Replace(b, []byte("\\u003c"), []byte("<"), -1)
	b = bytes.Replace(b, []byte("\\u003e"), []byte(">"), -1)
	b = bytes.Replace(b, []byte("\\u0026"), []byte("&"), -1)
	b = bytes.Replace(b, []byte("/"), []byte("\\/"), -1)

	return b, nil
}

type UserInfo struct {
	Login string
	RealName string
}

type UsersMap map[string]*UserInfo

func dumpUsers(api *slack.Client, dir string, requestedUsers []string, textOutput bool) UsersMap {
	fmt.Println("dump user information")
	users, err := api.GetUsers()
	check(err)

	data, err := MarshalIndent(users, "", "    ")
	check(err)
	err = ioutil.WriteFile(path.Join(dir, "users.json"), data, 0644)
	check(err)

	fmt.Println("dump direct message")
	ims, err := api.GetIMChannels()
	//fmt.Println(ims)

	var usersToDump [] slack.User

	if len(requestedUsers) > 0 && requestedUsers[0] != "@" {
		usersToDump = FilterUsers(users, func(user slack.User) bool {
			for _, rUser := range requestedUsers {
				if rUser == user.Name {
					return true
				}
			}
			return false
		})
	} else {
		usersToDump = users
	}

	usersMap := make(UsersMap)
	for _, user := range users {
		usersMap[user.ID] = &UserInfo { user.Name, user.RealName }
	}

	for _, im := range ims {
		for _, user := range usersToDump {
			if im.User == user.ID{
				fmt.Println("dump DM with " + user.Name)
				dumpChannel(api, dir, im.ID, user.Name, "dm", usersMap, textOutput)
			}
		}
	}

	return usersMap
}

func dumpRooms(api *slack.Client, dir string, rooms []string, usersMap UsersMap, textOutput bool) {
	// Dump Channels
	fmt.Println("dump public channel")
	channels := dumpChannels(api, dir, rooms, usersMap, textOutput)

	// Dump Private Groups
	fmt.Println("dump private channel")
	groups := dumpGroups(api, dir, rooms, usersMap, textOutput)

	if len(groups) > 0 {
		for _, group := range groups {
			channel := slack.Channel{}
			channel.ID = group.ID
			channel.Name = group.Name
			channel.Created = group.Created
			channel.Creator = group.Creator
			channel.IsArchived = group.IsArchived
			channel.IsChannel = true
			channel.IsGeneral = false
			channel.IsMember = true
			channel.LastRead = group.LastRead
			channel.Latest = group.Latest
			channel.Members = group.Members
			channel.Purpose = group.Purpose
			channel.Topic = group.Topic
			channel.UnreadCount = group.UnreadCount
			channel.UnreadCountDisplay = group.UnreadCountDisplay
			channels = append(channels, channel)
		}
	}

	data, err := MarshalIndent(channels, "", "    ")
	check(err)
	err = ioutil.WriteFile(path.Join(dir, "channels.json"), data, 0644)
	check(err)
}

func dumpChannels(api *slack.Client, dir string, rooms []string, usersMap UsersMap, textOutput bool) []slack.Channel {
	channels, err := api.GetChannels(false)
	check(err)

	if len(rooms) > 0 {
		channels = FilterChannels(channels, func(channel slack.Channel) bool {
			for _, room := range rooms {
				if room == channel.Name {
					return true
				}
			}
			return false
		})
	}

	if len(channels) == 0 {
		var channels []slack.Channel
		return channels
	}

	for _, channel := range channels {
		fmt.Println("dump channel " + channel.Name)
		dumpChannel(api, dir, channel.ID, channel.Name, "channel", usersMap, textOutput)
	}

	return channels
}

func dumpGroups(api *slack.Client, dir string, rooms []string, usersMap UsersMap, textOutput bool) []slack.Group {
	groups, err := api.GetGroups(false)
	check(err)
	if len(rooms) > 0 {
		groups = FilterGroups(groups, func(group slack.Group) bool {
			for _, room := range rooms {
				if room == group.Name {
					return true
				}
			}
			return false
		})
	}

	if len(groups) == 0 {
		var groups []slack.Group
		return groups
	}

	for _, group := range groups {
		fmt.Println("dump channel " + group.Name)
		dumpChannel(api, dir, group.ID, group.Name, "group", usersMap, textOutput)
	}

	return groups
}

func dumpChannel(api *slack.Client, dir, id, name, channelType string, usersMap UsersMap, textOutput bool) {
	var messages []slack.Message
	var channelPath string
	if channelType == "group" {
		channelPath = "private_channel"
		messages = fetchGroupHistory(api, id)
	} else if channelType == "dm" {
		channelPath = "direct_message"
		messages = fetchDirectMessageHistory(api, id)
	} else {
		channelPath = "channel"
		messages = fetchChannelHistory(api, id)
	}

	if len(messages) == 0 {
		return
	}

	sort.Sort(byTimestamp(messages))

	writeMessagesFile(messages, dir, channelPath, name, usersMap, textOutput)
}

var mentionRE = regexp.MustCompile("<@[0-9A-Z]+>")

func sameDay(t1, t2 *time.Time) bool {
	return t1.Year() == t2.Year() && t1.YearDay() == t2.YearDay()
}

func writeMessagesFile(messages []slack.Message, dir string, channelPath string, filename string, usersMap UsersMap,
	                   textOutput bool) {
	if len(messages) == 0 || dir == "" || channelPath == "" || filename == "" {
		return
	}
	channelDir := path.Join(dir, channelPath)
	err := os.MkdirAll(channelDir, 0755)
	check(err)

	var data []byte

	if textOutput {
		sdata := ""
		lastTimestamp := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
		for _, msg := range messages {
			timestamp := parseTimestamp(msg.Timestamp)
			if !sameDay(timestamp, &lastTimestamp) {
				sdata += fmt.Sprintf("\n----------------   %s    ----------------\n",
					                 timestamp.Format("Monday, Jan 2 2006"))
			}
			lastTimestamp = *timestamp

			userName, foundUser := usersMap[msg.User]
			if !foundUser { userName = &UserInfo{ msg.User, msg.User} }
			text := mentionRE.ReplaceAllStringFunc(msg.Text, func (t string) string {
				userName, foundUser := usersMap[t[2:len(t)-1]]
				if !foundUser { userName = &UserInfo{ msg.User, msg.User} }
				if msg.SubType != "" {
					return fmt.Sprintf("%s", userName.RealName)
				} else {
					return fmt.Sprintf("@%s", userName.Login)
				}
			})
			if msg.SubType == "" {
				sdata += fmt.Sprintf("[%s] %s: %s\n", timestamp.Format("15:04:05"), userName.RealName, text)
			} else {
				sdata += fmt.Sprintf("[%s] %s\n", timestamp.Format("15:04:05"), text)
			}
		}

		err = ioutil.WriteFile(path.Join(channelDir, filename + ".txt"), []byte(sdata), 0644)
		check(err)
	}


	data, err = MarshalIndent(messages, "", "    ")
	check(err)

	err = ioutil.WriteFile(path.Join(channelDir, filename + ".json"), data, 0644)
	check(err)
}

func fetchGroupHistory(api *slack.Client, ID string) []slack.Message {
	historyParams := slack.NewHistoryParameters()
	historyParams.Count = 1000

	// Fetch History
	history, err := api.GetGroupHistory(ID, historyParams)
	check(err)
	messages := history.Messages
	latest := messages[len(messages)-1].Timestamp
	for {
		if history.HasMore != true {
			break
		}

		historyParams.Latest = latest
		history, err = api.GetGroupHistory(ID, historyParams)
		check(err)
		length := len(history.Messages)
		if length > 0 {
			latest = history.Messages[length-1].Timestamp
			messages = append(messages, history.Messages...)
		}

	}

	return messages
}

func fetchChannelHistory(api *slack.Client, ID string) []slack.Message {
	historyParams := slack.NewHistoryParameters()
	historyParams.Count = 1000

	// Fetch History
	history, err := api.GetChannelHistory(ID, historyParams)
	check(err)
	messages := history.Messages
	latest := messages[len(messages)-1].Timestamp
	for {
		if history.HasMore != true {
			break
		}

		historyParams.Latest = latest
		history, err = api.GetChannelHistory(ID, historyParams)
		check(err)
		length := len(history.Messages)
		if length > 0 {
			latest = history.Messages[length-1].Timestamp
			messages = append(messages, history.Messages...)
		}

	}

	return messages
}

func fetchDirectMessageHistory(api *slack.Client, ID string) []slack.Message {
	historyParams := slack.NewHistoryParameters()
	historyParams.Count = 1000

	// Fetch History
	history, err := api.GetIMHistory(ID, historyParams)
	check(err)
	messages := history.Messages
	if len(messages) == 0 {
		return messages
	}
	latest := messages[len(messages)-1].Timestamp
	for {
		if history.HasMore != true {
			break
		}

		historyParams.Latest = latest
		history, err = api.GetIMHistory(ID, historyParams)
		check(err)
		length := len(history.Messages)
		if length > 0 {
			latest = history.Messages[length-1].Timestamp
			messages = append(messages, history.Messages...)
		}

	}

	return messages
}

func parseTimestamp(timestamp string) *time.Time {
	if utf8.RuneCountInString(timestamp) <= 0 {
		return nil
	}

	ts := timestamp

	if strings.Contains(timestamp, ".") {
		e := strings.Split(timestamp, ".")
		if len(e) != 2 {
			return nil
		}
		ts = e[0]
	}

	i, err := strconv.ParseInt(ts, 10, 64)
	check(err)
	tm := time.Unix(i, 0).Local()
	return &tm
}

// FilterGroups returns a new slice holding only
// the elements of s that satisfy f()
func FilterGroups(s []slack.Group, fn func(slack.Group) bool) []slack.Group {
	var p []slack.Group // == nil
	for _, v := range s {
		if fn(v) {
			p = append(p, v)
		}
	}
	return p
}

// FilterChannels returns a new slice holding only
// the elements of s that satisfy f()
func FilterChannels(s []slack.Channel, fn func(slack.Channel) bool) []slack.Channel {
	var p []slack.Channel // == nil
	for _, v := range s {
		if fn(v) {
			p = append(p, v)
		}
	}
	return p
}

// FilterUsers returns a new slice holding only
// the elements of s that satisfy f()
func FilterUsers(s []slack.User, fn func(slack.User) bool) []slack.User {
	var p []slack.User // == nil
	for _, v := range s {
		if fn(v) {
			p = append(p, v)
		}
	}
	return p
}
