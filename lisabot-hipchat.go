package main

import (
	"flag"
	"fmt"
	"github.com/ecwws/lisabot/lisaclient"
	"github.com/ecwws/lisabot/logging"
	"os"
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
	mentionNames  map[string]string
	xmpp          *xmppConn
	receivedUsers chan []*hipchatUser
	// receivedRooms   chan []*Room
	receivedMessage chan *message
	rooms           []Room
	host            string
	jid             string
	local_id        string
	apiHost         string
	chatHost        string
	mucHost         string
	webHost         string
}

type message struct {
	From        string
	To          string
	Body        string
	MentionName string
}

type hipchatUser struct {
	Id          string
	Name        string
	MentionName string
}

type xmppMessage struct {
	Type     string `xml:"type,attr"`
	From     string `xml:"from,attr"`
	Body     string `xml:"body"`
	RoomName string `xml:"x>name"`
	RoomId   string `xml:"x>id"`
}

var logger *logging.LisaLog

func main() {

	user := flag.String("user", "", "hipchat username")
	pass := flag.String("pass", "", "hipchat password")
	nick := flag.String("nick", "Lisa Bot", "hipchat full name")
	server := flag.String("server", "127.0.0.1", "lisabot server")
	port := flag.String("port", "4517", "lisabot server port")
	sourceid := flag.String("id", "lisabot-hipchat", "source id")
	loglevel := flag.String("loglevel", "warn", "loglevel")

	flag.Parse()

	var err error

	logger, err = logging.NewLogger(os.Stdout, *loglevel)

	if err != nil {
		fmt.Println("Error initializing logger: ", err)
		os.Exit(-1)
	}

	conn, err := xmppConnect(hipchatHost)

	if err != nil {
		logger.Error.Println("Error connecting to lisabot:", err)
		os.Exit(1)
	}

	logger.Info.Println("Connected")

	hc := &hipchatClient{
		username: *user,
		password: *pass,
		resource: "bot",
		id:       *user + "@" + hipchatHost,
		nick:     *nick,

		xmpp:            conn,
		mentionNames:    make(map[string]string),
		receivedUsers:   make(chan []*hipchatUser),
		receivedMessage: make(chan *message),
		host:            hipchatHost,
	}

	err = hc.initialize()

	if err != nil {
		panic(err)
	}
	logger.Info.Println("Authenticated")

	lisa, err := lisaclient.NewClient(*server, *port)

	if err != nil {
		logger.Error.Println("Failed to create lisabot-hipchate:", err)
		os.Exit(2)
	}

	err = lisa.Engage("adapter", *sourceid)

	if err != nil {
		logger.Error.Println("Failed to engage:", err)
		os.Exit(3)
	}

	logger.Info.Println("LisaBot engaged")

	// quit := make(chan int)

	hc.rooms = hc.xmpp.Discover(hc.jid, hc.mucHost)
	hc.xmpp.Join(hc.jid, hc.nick, hc.rooms)
	hc.xmpp.Available(hc.jid)

	run(lisa, hc)
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
				c.local_id = strings.Split(c.jid, "_")[0]
				c.apiHost = info.ApiHost
				c.chatHost = info.ChatHost
				c.mucHost = info.MucHost
				c.webHost = info.WebHost
				logger.Debug.Println("JID:", c.jid)
				return nil
			}
		case "proceed" + xmppNsTLS:
			c.xmpp.UseTLS(c.host)
			c.xmpp.StreamStart(c.id, c.host)
		}

	}
	return nil
}

func (c *hipchatClient) keepAlive(trigger chan<- bool) {
	for _ = range time.Tick(60 * time.Second) {
		trigger <- true
	}
}

func run(lisa *lisaclient.LisaClient, hc *hipchatClient) {
	messageFromHC := make(chan *xmppMessage)
	go hc.listen(messageFromHC)

	fromLisa := make(chan *lisaclient.Query)
	toLisa := make(chan *lisaclient.Query)
	go lisa.Run(toLisa, fromLisa)

	keepAlive := make(chan bool)
	go hc.keepAlive(keepAlive)

	for {
		select {
		case msg := <-messageFromHC:
			logger.Debug.Println("Type:", msg.Type)
			logger.Debug.Println("From:", msg.From)
			logger.Debug.Println("Message:", msg.Body)
			if msg.Body != "" {
				toLisa <- &lisaclient.Query{
					Type:   "message",
					Source: lisa.SourceId,
					Message: &lisaclient.MessageBlock{
						Message: msg.Body,
						From:    msg.From,
					},
				}
			} else if msg.RoomName != "" {
				hc.xmpp.Join(hc.jid, hc.nick, []Room{
					Room{
						Id: hc.local_id + "_" + msg.RoomName + "@" + hipchatConf,
					},
				})
			}
		case query := <-fromLisa:
			logger.Debug.Println("Query type:", query.Type)
		case <-keepAlive:
			hc.xmpp.KeepAlive()
			logger.Debug.Println("KeepAlive sent")
		}
	}
}

func (c *hipchatClient) listen(msgChan chan<- *xmppMessage) {

	if logger.Level == "debug" {
		c.xmpp.Debug()
	}
	for {
		element, err := c.xmpp.RecvNext()

		if err != nil {
			continue
		}

		switch element.Name.Local {
		case "message":
			message := new(xmppMessage)
			c.xmpp.DecodeElement(message, &element)
			msgChan <- message

			logger.Debug.Println(*message)
		default:
			c.xmpp.Skip()
		}

	}
}
