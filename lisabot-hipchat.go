package main

import (
	"flag"
	"fmt"
	"github.com/ecwws/lisabot-hipchat/xmpp"
	"github.com/ecwws/lisabot/lisaclient"
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
	xmpp          *xmpp.Conn
	receivedUsers chan []*hipchatUser
	// receivedRooms   chan []*Room
	receivedMessage chan *message
	rooms           []xmpp.Room
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

func main() {

	user := flag.String("user", "", "hipchat username")
	pass := flag.String("pass", "", "hipchat password")
	nick := flag.String("nick", "Lisa Bot", "hipchat full name")
	server := flag.String("server", "127.0.0.1", "lisabot server")
	port := flag.String("port", "4517", "lisabot server port")

	flag.Parse()

	conn, err := xmpp.Connect(hipchatHost)

	if err != nil {
		fmt.Println("Error: ", err.Error())
		os.Exit(1)
	}
	fmt.Println("Connected")

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
	fmt.Println("Authenticated")

	lisa, err := lisaclient.NewClient(*server, *port)
	if err != nil {
		panic(err)
	}
	lisa.Engage()
	fmt.Println("LisaBot connected")

	hc.rooms = hc.xmpp.Discover(hc.jid, hc.mucHost)

	hc.xmpp.Join(hc.jid, hc.nick, hc.rooms)

	hc.xmpp.Available(hc.jid)

	quit := make(chan int)
	go hc.listen(quit)
	go hc.keepAlive()

	<-quit
}

func (c *hipchatClient) initialize() error {
	c.xmpp.StreamStart(c.id, c.host)
	for {
		element, err := c.xmpp.RecvNext()

		if err != nil {
			return err
		}

		switch element.Name.Local + element.Name.Space {
		case "stream" + xmpp.NsStream:
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
		case "proceed" + xmpp.NsTLS:
			c.xmpp.UseTLS(c.host)
			c.xmpp.StreamStart(c.id, c.host)
		}

	}
	return nil
}

func (c *hipchatClient) keepAlive() {
	for _ = range time.Tick(60 * time.Second) {
		c.xmpp.KeepAlive()
	}
}

func (c *hipchatClient) listen(quit chan int) {
	// c.xmpp.ReadRaw()
	for {
		element, err := c.xmpp.RecvNext()

		if err != nil {
			continue
		}

		var message xmppMessage

		switch element.Name.Local {
		case "message":
			c.xmpp.DecodeElement(&message, &element)
			fmt.Println("Type: ", message.Type)
			fmt.Println("From: ", message.From)
			fmt.Println("Message: ", message.Body)
		default:
			c.xmpp.Skip()
		}

	}
}
