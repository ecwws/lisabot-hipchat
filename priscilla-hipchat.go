package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"github.com/priscillachat/prisclient"
	"github.com/priscillachat/prislog"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	hipchatHost = "chat.hipchat.com"
	hipchatConf = "conf.hipchat.com"
)

type hipchatClient struct {
	username string
	password string
	resource string
	id       string
	nick     string

	// private
	usersByMention  map[string]*hipchatUser
	usersByName     map[string]*hipchatUser
	usersByJid      map[string]*hipchatUser
	usersByEmail    map[string]*hipchatUser
	xmpp            *xmppConn
	receivedMessage chan *message
	roomsByName     map[string]string
	roomsById       map[string]string
	host            string
	jid             string
	accountId       string
	apiHost         string
	chatHost        string
	mucHost         string
	webHost         string
	token           string
	mention         string
	aMention        string
}

type message struct {
	From        string
	To          string
	Body        string
	MentionName string
}

type xmppMessage struct {
	XMLName  xml.Name `xml:"message"`
	Type     string   `xml:"type,attr"`
	From     string   `xml:"from,attr"`
	FromJid  string   `xml:"from_jid,attr"`
	To       string   `xml:"to,attr"`
	Id       string   `xml:"id,attr"`
	Body     string   `xml:"body"`
	RoomName string   `xml:"x>name,omitempty"`
	RoomId   string   `xml:"x>id,omitempty"`
}

var logger *prislog.PrisLog

func main() {

	user := flag.String("user", "", "hipchat username")
	pass := flag.String("pass", "", "hipchat password")
	nick := flag.String("nick", "Priscilla", "hipchat full name")
	server := flag.String("server", "127.0.0.1", "priscilla server")
	port := flag.String("port", "4517", "priscilla server port")
	sourceid := flag.String("id", "priscilla-hipchat", "source id")
	loglevel := flag.String("loglevel", "warn", "loglevel")

	flag.Parse()

	var err error

	logger, err = prislog.NewLogger(os.Stdout, *loglevel)

	if err != nil {
		fmt.Println("Error initializing logger: ", err)
		os.Exit(-1)
	}

	hc := &hipchatClient{
		username: *user,
		password: *pass,
		resource: "bot",
		id:       *user + "@" + hipchatHost,
		nick:     *nick,

		xmpp:            nil,
		usersByMention:  make(map[string]*hipchatUser),
		usersByJid:      make(map[string]*hipchatUser),
		usersByName:     make(map[string]*hipchatUser),
		usersByEmail:    make(map[string]*hipchatUser),
		receivedMessage: make(chan *message),
		host:            hipchatHost,
		roomsByName:     make(map[string]string),
		roomsById:       make(map[string]string),
	}

	priscilla, err :=
		prisclient.NewClient(*server, *port, "adapter", *sourceid, true, logger)

	if err != nil {
		logger.Error.Println("Failed to create priscilla-hipchate:", err)
		os.Exit(2)
	}

	// quit := make(chan int)

	run(priscilla, hc)
	// go hc.keepAlive()

	// <-quit
}

func (c *hipchatClient) initialize() error {
	c.xmpp.StreamStart(c.id, c.host)
	for {
		element, err := c.xmpp.RecvNext()

		if err != nil {
			return err
		}

		switch element.Name.Local + element.Name.Space {
		case "stream" + xmppNsStream:
			features := c.xmpp.RecvFeatures()
			if features.StartTLS != nil {
				c.xmpp.StartTLS()
			} else {
				info, err := c.xmpp.Auth(c.username, c.password, c.resource)
				if err != nil {
					return err
				}
				c.jid = info.Jid
				c.accountId = strings.Split(c.jid, "_")[0]
				c.apiHost = info.ApiHost
				c.chatHost = info.ChatHost
				c.mucHost = info.MucHost
				c.webHost = info.WebHost
				c.token = info.Token
				// c.tokenExp = time.Now().Unix() + 2592000
				logger.Debug.Println("JID:", c.jid)
				logger.Debug.Println("Token:", info.Token)
				return nil
			}
		case "proceed" + xmppNsTLS:
			c.xmpp.UseTLS(c.host)
			c.xmpp.StreamStart(c.id, c.host)
			if logger.Level == "debug" {
				c.xmpp.Debug()
			}
		}

	}
	return nil
}

func (c *hipchatClient) keepAlive(trigger chan<- bool) {
	for _ = range time.Tick(60 * time.Second) {
		trigger <- true
	}
}

func run(priscilla *prisclient.Client, hc *hipchatClient) {

	messageFromHC := make(chan *xmppMessage)
	go hc.listen(messageFromHC)

	fromPris := make(chan *prisclient.Query)
	toPris := make(chan *prisclient.Query)
	go priscilla.Run(toPris, fromPris)

	keepAlive := make(chan bool)
	go hc.keepAlive(keepAlive)

mainLoop:
	for {
		select {
		case msg := <-messageFromHC:
			logger.Debug.Println("Type:", msg.Type)
			logger.Debug.Println("From:", msg.From)
			logger.Debug.Println("Message:", msg.Body)
			logger.Debug.Println("Room Invite:", msg.RoomName)

			fromSplit := strings.Split(msg.From, "/")
			fromRoom := fromSplit[0]
			var fromNick string
			if len(fromSplit) > 1 {
				fromNick = fromSplit[1]
			}

			if msg.FromJid != "" {
				if _, exist := hc.usersByJid[msg.FromJid]; !exist {
					hc.xmpp.VCardRequest(hc.jid, msg.FromJid)
				}
			}

			if msg.Body != "" && fromNick != hc.nick {
				mentioned, err := regexp.MatchString("@"+hc.mention, msg.Body)
				if err != nil {
					logger.Error.Println("Error searching for mention:", err)
				}

				clientQuery := prisclient.Query{
					Type:   "message",
					Source: priscilla.SourceId,
					To:     "server",
					Message: &prisclient.MessageBlock{
						Message:   msg.Body,
						From:      fromNick,
						Room:      hc.roomsById[fromRoom],
						Mentioned: mentioned,
						Stripped: strings.Replace(msg.Body, hc.aMention,
							"", -1),
					},
				}

				if mentioned {
					clientQuery.Message.Message = strings.Replace(msg.Body,
						"@"+hc.mention, "", -1)
				}

				toPris <- &clientQuery
			} else if msg.RoomName != "" {
				hc.roomsByName[msg.RoomName] = msg.From
				hc.roomsById[msg.From] = msg.RoomName
				hc.xmpp.Join(hc.jid, hc.nick, []string{msg.From})
			}
		case query := <-fromPris:
			logger.Debug.Println("Query received:", *query)
			switch {
			case query.Type == "command" &&
				query.Command.Action == "disengage":
				// either server forcing disengage or server connection lost
				logger.Warn.Println("Disengage received, terminating...")
				break mainLoop
			case query.Type == "message":
				hc.groupMessage(query.Message)
				// hc.groupMessage(hc.roomsByName[query.Message.Room],
				//  query.Message.Message)
			}
		case <-keepAlive:
			hc.xmpp.KeepAlive()
			logger.Debug.Println("KeepAlive sent")
			// within 60 seconds of token expiration
			// if hc.tokenExp < time.Now().Unix()+60 {
			// if true {
			//  hc.xmpp.AuthRequest(hc.username, hc.password, hc.resource)
			//  logger.Info.Println("New token requested")
			// }
		}
	}
}

func (c *hipchatClient) groupMessage(message *prisclient.MessageBlock) error {

	xmppMsg := xmppMessage{
		From: c.jid,
		To:   c.roomsByName[message.Room] + "/" + c.nick,
		Id:   prisclient.RandomId(),
		Type: "groupchat",
		Body: message.Message,
	}

	if len(message.MentionNotify) > 0 {
		for _, name := range message.MentionNotify {
			if user, ok := c.usersByName[name]; ok {
				xmppMsg.Body += " @" + user.Mention
			}
		}
	}
	return c.xmpp.Encode(&xmppMsg)
}

func (c *hipchatClient) establishConnection() error {
	var err error

	c.xmpp, err = xmppConnect(hipchatHost)

	if err != nil {
		logger.Error.Println("Error connecting to hipchat:", err)
		return err
	}

	logger.Info.Println("Connected to HipChat")

	err = c.initialize()

	if err != nil {
		logger.Error.Println("Failed to initialize HipChat connection:", err)
		return err
	}
	logger.Info.Println("Authenticated")

	c.xmpp.VCardRequest(c.jid, "")
	self, err := c.xmpp.VCardDecode(nil)

	if err != nil {
		logger.Error.Println("Failed to retrieve info on myself:", err)
		return err
	}

	c.mention = self.Mention
	c.aMention = "@" + self.Mention

	self.Jid = c.jid

	c.updateUserInfo(self)

	rooms := c.xmpp.Discover(c.jid, c.mucHost)

	autojoin := make([]string, 0, len(rooms))

	for _, room := range rooms {
		c.roomsByName[room.Name] = room.Id
		c.roomsById[room.Id] = room.Name
		autojoin = append(autojoin, room.Id)
	}

	c.xmpp.Join(c.jid, c.nick, autojoin)
	c.xmpp.Available(c.jid)

	return nil
}

func (c *hipchatClient) listen(msgChan chan<- *xmppMessage) {

	for err := c.establishConnection(); err != nil; err = c.establishConnection() {
		logger.Error.Println("Failed to establish connection with hipchat:", err)
		logger.Warn.Println("Sleeping 10 seconds before retry...")
		c.xmpp.Disconnect()
		time.Sleep(10 * time.Second)
	}

	for {
		element, err := c.xmpp.RecvNext()

		if err != nil {
			logger.Error.Println(err)
			if err.Error() == "EOF" {
				for err := c.establishConnection(); err != nil; err = c.establishConnection() {
					logger.Error.Println(
						"Failed to establish connection with hipchat:", err)
					logger.Warn.Println("Sleeping 10 seconds before retry...")
					c.xmpp.Disconnect()
					time.Sleep(10 * time.Second)
				}
			}
			continue
		}

		switch element.Name.Local {
		case "message":
			message := new(xmppMessage)
			c.xmpp.DecodeElement(message, &element)
			msgChan <- message

			logger.Debug.Println(*message)
		case "iq":
			userInfo, err := c.xmpp.VCardDecode(&element)
			if err == nil {
				c.updateUserInfo(userInfo)
			} else {
				logger.Error.Println("Error decoding user vCard:", err)
			}
		// case "success":
		//  var auth authResponse
		//  c.xmpp.AuthResp(&auth, &element)
		//  if auth.Token != "" {
		//   c.token = auth.Token
		//   c.tokenExp = time.Now().Unix() + 2592000
		//   logger.Debug.Println("New token:", c.token)
		//  }
		default:
			c.xmpp.Skip()
		}

	}
}

func (c *hipchatClient) updateUserInfo(info *hipchatUser) {
	c.usersByMention[info.Mention] = info
	c.usersByJid[info.Jid] = info
	c.usersByName[info.Name] = info
	c.usersByEmail[info.Email] = info

	logger.Debug.Println("User info obtained:", *info)
}
