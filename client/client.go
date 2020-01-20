package main

import (
	"bufio"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	gopenpgp "github.com/ProtonMail/gopenpgp/crypto"
	"github.com/labstack/gommon/log"
	"github.com/syleron/426c/common/models"
	plib "github.com/syleron/426c/common/packet"
	"github.com/syleron/426c/common/utils"
	"net"
	"strings"
)

type Client struct {
	Reader *bufio.Reader
	Writer *bufio.Writer
	Conn   net.Conn
	MQ     *MessageQueue
}

func setupClient() (*Client, error) {
	// Setup our listener
	cert, err := tls.LoadX509KeyPair("cert.pem", "key.pem")
	if err != nil {
		panic(err)
	}
	config := tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
	}
	config.Rand = rand.Reader
	// connect to this socket
	// TODO This should be a client command rather done automagically.
	conn, err := tls.Dial("tcp", "127.0.0.1:9000", &config)
	if err != nil {
		return &Client{}, errors.New("unable to connect to host")
	}
	// Put our handlers into a go routine
	c := &Client{
		Writer: bufio.NewWriter(conn),
		Reader: bufio.NewReader(conn),
		Conn:   conn,
		MQ:     NewMessageQueue(),
	}
	// Put our handlers into a go routine
	go c.connectionHandler()
	return c, nil
}

func (c *Client) Send(cmdType int, buf []byte) (int, error) {
	return c.Conn.Write(plib.PacketForm(byte(cmdType), buf))
}

func (c *Client) connectionHandler() {
	for {
		p, err := plib.PacketRead(c.Reader)
		if err != nil {
			app.Stop()
		}
		c.commandRouter(p)
	}
}

func (c *Client) commandRouter(p []byte) {
	switch p[0] {
	case plib.SVR_LOGIN:
		c.svrLogin(p[1:])
	case plib.SVR_USER:
		c.svrUser(p[1:])
	default:
	}
}

// ||
// Client Requests
// ||

func (c *Client) cmdRegister(username string, password string) {
	var pgp = gopenpgp.GetGopenPGP()
	// Generate password hash
	hashString := hashPassword(password)
	// Calculate hash key
	hashKey := hashString[:32]
	// Calculate hash remainder
	hashRemainder := hashString[32:48]
	// Generate RSA key
	rsaKey, err := pgp.GenerateKey(
		username,
		"secure.426c.net",
		hashString,
		"rsa",
		4096,
	)
	// TODO: Save our RSA Key
	if err := utils.WriteFile(rsaKey, username); err != nil{
		panic(err)
	}
	if err != nil {
		panic(err)
	}
	keyRing, err := gopenpgp.ReadArmoredKeyRing(strings.NewReader(rsaKey))
	if err != nil {
		panic(err)
	}
	publicKey, err := keyRing.GetArmoredPublicKey()
	if err != nil {
		panic(err)
	}
	// Encrypt our private RSA key
	encryptedKey, err := encryptRSA([]byte(rsaKey), []byte(hashRemainder), []byte(hashKey))
	if err != nil {
		panic(err)
	}
	// Create our object to send
	registerObject := &models.RegisterRequestModel{
		Username:   username,
		PassHash:   hashRemainder,
		EncPrivKey: encryptedKey,
		PubKey:     publicKey,
	}
	// Send our username, hash remainder, encrypted private key, and readable public key.
	_, err = c.Send(
		plib.CMD_REGISTER,
		utils.MarshalResponse(registerObject),
	)
	if err != nil {
		panic(err)
	}
}

func (c *Client) cmdLogin(username string, password string) {
	// Generate password hash
	hashString := hashPassword(password)
	// Calculate hash remainder
	hashRemainder := hashString[32:48]
	// Create our object to send
	registerObject := &models.LoginRequestModel{
		Username: username,
		Password: hashRemainder,
		Version:  VERSION,
	}
	// Send our username, hash remainder.
	_, err := c.Send(
		plib.CMD_LOGIN,
		utils.MarshalResponse(registerObject),
	)
	if err != nil {
		panic(err)
	}
}

// cmdMsgTo - Send a private encrypted message to a particular user
// TODO: Successful/failed messages need to be marked in our local DB
// Process:
// 1) Send cmd_user request in attempt to ensure user exists.
// 2)
func (c *Client) cmdMsgTo(m *models.Message) {
	// We need to get our recipients deets so we can encrypt our message
	_, err := c.Send(plib.CMD_USER, utils.MarshalResponse(&models.UserRequestModel{
		Username: m.To,
	}))
	if err != nil {
		panic(err)
	}
}

// ||
// Server Responses
// ||

func (c *Client) svrRegister(p []byte) error {
	var regObj models.RegisterResponseModel
	if err := json.Unmarshal(p, &regObj); err != nil {
		panic("unable to unmarshal packet")
	}
	if !regObj.Success {
		showError(ClientError{
			Message: regObj.Message,
			Button:  "Continue",
			Continue: func() {
				pages.SwitchToPage("login")
			},
		})
		return nil
	}
	return nil
}

func (c *Client) svrLogin(p []byte) {
	var loginObj models.LoginResponseModel
	if err := json.Unmarshal(p, &loginObj); err != nil {
		log.Debug("unable to unmarshal packet")
		return
	}
	// Make sure our response object was successful
	if !loginObj.Success {
		showError(ClientError{
			Message: loginObj.Message,
			Button:  "Continue",
			Continue: func() {
				pages.SwitchToPage("login")
			},
		})
		return
	}
	// Load our private key
	b, err := utils.LoadFile(loginObj.Username)
	if err != nil {
		showError(ClientError{
			Message: "Login failed. Unable to load private key for " + loginObj.Username + ".",
			Button:  "Continue",
		})
		return
	}
	// Set our private key
	privKey = string(b)
	// Success, switch pages
	pages.SwitchToPage("inbox")
}

func (c *Client) svrMsgTo() {

}

// svrUser - User Object response from network and update our local DB
func (c *Client) svrUser(p []byte) {
	var userObj models.UserResponseModel
	if err := json.Unmarshal(p, &userObj); err != nil {
		log.Debug("unable to unmarshal packet")
		return
	}
	if !userObj.Success {
		showError(ClientError{
			Message:  userObj.Message,
			Button:   "Continue",
			Continue: nil,
		})
		return
	}
	// Insert our user into our local DB
	dbUserAdd(userObj.User)
}

func (c *Client) Close() {}
