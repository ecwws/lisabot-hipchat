package main

import (
	"flag"
	"fmt"
	"github.com/ecwws/lisabot/lisaclient"
	"github.com/ecwws/lisabot/logging"
	"os"
	"time"
)

const (
	hipchatHost = "chat.hipchat.com"
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
	Type string `xml:"type,attr"`
	From string `xml:"from,attr"`
	Body string `xml:"body"`
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
				c.apiHost = info.ApiHost
				c.chatHost = info.ChatHost
				c.mucHost = info.MucHost
				c.webHost = info.WebHost
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
	fromHC := make(chan *xmppMessage)
	go hc.listen(fromHC)

	fromLisa := make(chan *lisaclient.Query)
	toLisa := make(chan *lisaclient.Query)
	go lisa.Run(toLisa, fromLisa)

	keepAlive := make(chan bool)
	go hc.keepAlive(keepAlive)

	for {
		select {
		case msg := <-fromHC:
			logger.Debug.Println("Type:", msg.Type)
			logger.Debug.Println("From:", msg.From)
			logger.Debug.Println("Message:", msg.Body)
		case query := <-fromLisa:
			logger.Debug.Println("Query type:", query.Type)
		case <-keepAlive:
			hc.xmpp.KeepAlive()
			logger.Debug.Println("KeepAlive sent")
		}
	}
}

func (c *hipchatClient) listen(msgChan chan<- *xmppMessage) {
	// c.xmpp.ReadRaw()
	for {
		element, err := c.xmpp.RecvNext()

		if err != nil {
			continue
		}

		message := new(xmppMessage)

		switch element.Name.Local {
		case "message":
			c.xmpp.DecodeElement(message, &element)
			msgChan <- message
		default:
			c.xmpp.Skip()
		}

	}
}
