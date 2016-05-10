package main

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
)

const (
	xmppNsStream   = "http://etherx.jabber.org/streams"
	xmppNsTLS      = "urn:ietf:params:xml:ns:xmpp-tls"
	xmppNsHipchat  = "http://hipchat.com"
	xmppNsDiscover = "http://jabber.org/protocol/disco#items"
	xmppNsMuc      = "http://jabber.org/protocol/muc"

	streamStart = `<stream:stream
		xmlns='jabber:client'
		xmlns:stream='http://etherx.jabber.org/streams'
		from='%s'
		to='%s'
		version='1.0'>`
	streamEnd = "</stream:stream>"
)

type xmppConn struct {
	raw     net.Conn
	decoder *xml.Decoder
	encoder *xml.Encoder
}

type emptyElement struct {
	XMLName xml.Name
}

type charElement struct {
	XMLName xml.Name
	Value   string `xml:,chardata`
}

type required struct{}

type features struct {
	XMLName    xml.Name  `xml:"features"`
	StartTLS   *required `xml:"starttls>required"`
	Mechanisms []string  `xml:"mechanisms>mechanism"`
}

type authResponse struct {
	XMLName  xml.Name `xml:"success"`
	Jid      string   `xml:"jid,attr"`
	ApiHost  string   `xml:"api_host,attr"`
	ChatHost string   `xml:"chat_host,attr"`
	MucHost  string   `xml:"muc_host,attr"`
	WebHost  string   `xml:"web_host,attr"`
}

type xmppIq struct {
	XMLName xml.Name `xml:"iq"`
	Type    string   `xml:"type,attr"`
	Id      string   `xml:"id,attr"`
	From    string   `xml:"from,attr"`
	To      string   `xml:"to,attr"`
	Query   interface{}
}

type xmppPresence struct {
	XMLName xml.Name `xml:"presence"`
	Id      string   `xml:"id,attr,omitempty"`
	From    string   `xml:"from,attr"`
	To      string   `xml:"to,attr,omitempty"`
	Status  interface{}
}

type xmppAuth struct {
	XMLName xml.Name `xml:"auth"`
	Ns      string   `xml:"xmlns,attr"`
	Value   string   `xml:",chardata"`
}

type xmppShow struct {
	XMLName xml.Name `xml:"show"`
	Value   string   `xml:",chardata"`
}

type Room struct {
	XMLName xml.Name `xml:"item"`
	Id      string   `xml:"jid,attr"`
	Name    string   `xml:"name,attr"`
}

type xmppDiscover struct {
	XMLName xml.Name `xml:"iq"`
	Rooms   []Room   `xml:"query>item"`
}

func xmppConnect(host string) (*xmppConn, error) {
	c := new(xmppConn)

	conn, err := net.Dial("tcp", host+":5222")

	if err != nil {
		return c, err
	}

	c.raw = conn
	c.decoder = xml.NewDecoder(c.raw)
	c.encoder = xml.NewEncoder(c.raw)

	return c, nil
}

func (c *xmppConn) StreamStart(id, host string) {
	fmt.Fprintf(c.raw, streamStart, id, host)
}

func (c *xmppConn) RecvNext() (element xml.StartElement, err error) {
	for {
		var t xml.Token
		t, err = c.decoder.Token()
		if err != nil {
			return element, err
		}

		switch t := t.(type) {
		case xml.StartElement:
			element = t
			if element.Name.Local == "" {
				err = errors.New("Bad XML response")
				return
			}

			return
		}
	}
}

func (c *xmppConn) RecvFeatures() *features {
	var f features
	err := c.decoder.Decode(&f)

	if err != nil {
		panic(err)
	}

	return &f
}

func (c *xmppConn) StartTLS() {
	starttls := emptyElement{
		XMLName: xml.Name{Local: "starttls", Space: xmppNsTLS},
	}
	c.encoder.Encode(starttls)
}

func (c *xmppConn) UseTLS(host string) {
	c.raw = tls.Client(c.raw, &tls.Config{ServerName: host})
	c.decoder = xml.NewDecoder(c.raw)
	c.encoder = xml.NewEncoder(c.raw)
}

func (c *xmppConn) Auth(username, password, resource string) (*authResponse, error) {
	token := []byte{'\x00'}

	token = append(token, []byte(username)...)
	token = append(token, '\x00')
	token = append(token, []byte(password)...)
	token = append(token, '\x00')
	token = append(token, []byte(resource)...)

	encodedToken := base64.StdEncoding.EncodeToString(token)

	auth := xmppAuth{
		Ns:    xmppNsHipchat,
		Value: encodedToken,
	}
	// out, _ := xml.Marshal(auth)
	// fmt.Println(string(out))
	c.encoder.Encode(auth)

	var response authResponse

	err := c.decoder.Decode(&response)

	return &response, err
}

func id() string {
	b := make([]byte, 8)
	io.ReadFull(rand.Reader, b)
	return fmt.Sprintf("%x", b)
}

func (c *xmppConn) Available(from string) {
	available := xmppPresence{
		Id:     id(),
		From:   from,
		Status: &xmppShow{Value: "chat"},
	}

	c.encoder.Encode(available)
}

func (c *xmppConn) Discover(from, to string) []Room {
	discover := xmppIq{
		Type: "get",
		Id:   id(),
		From: from,
		To:   to,
		Query: &emptyElement{
			XMLName: xml.Name{Local: "query", Space: xmppNsDiscover},
		},
	}

	c.encoder.Encode(discover)

	var result xmppDiscover
	err := c.decoder.Decode(&result)

	if err != nil {
		panic(err)
	}

	return result.Rooms
}

func (c *xmppConn) KeepAlive() {
	fmt.Fprintf(c.raw, " ")
}

func (c *xmppConn) ReadRaw() {
	for {
		buf := make([]byte, 128)
		count, _ := c.raw.Read(buf)

		logger.Debug.Print(string(buf[:count]))
	}
}

func (c *xmppConn) Skip() error {
	return c.decoder.Skip()
}

func (c *xmppConn) DecodeElement(v interface{}, start *xml.StartElement) error {
	return c.decoder.DecodeElement(v, start)
}

func (c *xmppConn) Join(from, nick string, rooms []Room) {
	for _, room := range rooms {
		join := xmppPresence{
			Id:   id(),
			From: from,
			To:   room.Id + "/" + nick,
			Status: &emptyElement{
				XMLName: xml.Name{Local: "x", Space: xmppNsMuc},
			},
		}
		out, _ := xml.Marshal(join)
		logger.Debug.Println("Request to join room:", string(out))
		c.encoder.Encode(join)
	}
}
