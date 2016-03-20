package main

import (
	"flag"
	"fmt"
	"github.com/ecwws/lisabot-hipchat/xmpp"
	"os"
	"time"
)

const (
	hipchatHost = "chat.hipchat.com"
)

type Client struct {
	username string
	password string
	resource string
	id       string

	// private
	mentionNames    map[string]string
	xmpp            *xmpp.Conn
	receivedUsers   chan []*User
	receivedRooms   chan []*Room
	receivedMessage chan *Message
	host            string
	jid             string
	apiHost         string
	chatHost        string
	mucHost         string
	webHost         string
}

type Message struct {
	From        string
	To          string
	Body        string
	MentionName string
}

type User struct {
	Id          string
	Name        string
	MentionName string
}

type Room struct {
	Id   string
	Name string
}

func main() {

	user := flag.String("user", "", "hipchat username")
	pass := flag.String("pass", "", "hipchat password")

	flag.Parse()

	conn, err := xmpp.Connect(hipchatHost)

	if err != nil {
		fmt.Println("Error: ", err.Error())
		os.Exit(1)
	}
	fmt.Println("Connected")

	c := &Client{
		username: *user,
		password: *pass,
		resource: "bot",
		id:       *user + "@" + hipchatHost,

		xmpp:            conn,
		mentionNames:    make(map[string]string),
		receivedUsers:   make(chan []*User),
		receivedRooms:   make(chan []*Room),
		receivedMessage: make(chan *Message),
		host:            hipchatHost,
	}

	err = c.initialize()

	if err != nil {
		panic(err)
	}

	fmt.Println("Authenticated")

	c.xmpp.Available(c.jid)

	quit := make(chan int)
	go c.listen(quit)
	go c.keepAlive()

	<-quit
}

func (c *Client) initialize() error {
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

func (c *Client) keepAlive() {
	for _ = range time.Tick(60 * time.Second) {
		c.xmpp.KeepAlive()
	}
}

func (c *Client) listen(quit chan int) {
	c.xmpp.ReadRaw()
}
