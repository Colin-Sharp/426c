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
	"github.com/syleron/426c/common/security"
	"github.com/syleron/426c/common/utils"
	"net"
	"strings"
)

type Client struct {
	Reader *bufio.Reader
	Writer *bufio.Writer
	Conn   net.Conn
	Username string
	Blocks int
}

func setupClient(address string) (*Client, error) {
	// Setup our listener
	cert, err := tls.LoadX509KeyPair("cert.pem", "key.pem")
	if err != nil {
		app.Stop()
	}
	config := tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
	}
	config.Rand = rand.Reader
	// connect to this socket
	// TODO This should be a client command rather done automagically.
	conn, err := tls.Dial("tcp", address, &config)
	if err != nil {
		return &Client{}, errors.New("unable to connect to host")
	}
	// Put our handlers into a go routine
	c := &Client{
		Writer: bufio.NewWriter(conn),
		Reader: bufio.NewReader(conn),
		Conn:   conn,
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
	if len(p) <= 0 {
		return
	}
	switch p[0] {
	case plib.SVR_LOGIN:
		c.svrLogin(p[1:])
	case plib.SVR_USER:
		c.svrUser(p[1:])
	case plib.SVR_MSGTO:
		c.svrMsgTo(p[1:])
	case plib.SVR_MSG:
		c.svrMsg(p[1:])
	case plib.SVR_BLOCK:
		c.svrBlock(p[1:])
	default:
		panic("balls")
	}
}

// ||
// Client Requests
// ||

func (c *Client) cmdRegister(username string, password string) {
	var pgp = gopenpgp.GetGopenPGP()
	// Generate password hash
	hashString := security.SHA512HashEncode(password)
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
	// save our key
	if err := utils.WriteFile(rsaKey, username); err != nil{
		app.Stop()
	}
	if err != nil {
		app.Stop()
	}
	keyRing, err := gopenpgp.ReadArmoredKeyRing(strings.NewReader(rsaKey))
	if err != nil {
		app.Stop()
	}
	publicKey, err := keyRing.GetArmoredPublicKey()
	if err != nil {
		app.Stop()
	}
	// Encrypt our private RSA key
	encryptedKey, err := security.EncryptRSA([]byte(rsaKey), []byte(hashRemainder), []byte(hashKey))
	if err != nil {
		app.Stop()
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
		app.Stop()
	}
}

func (c *Client) cmdLogin(username string, password string) {
	// Generate password hash
	hashString := security.SHA512HashEncode(password)
	// Calculate hash remainder
	hashRemainder := hashString[32:48]
	// Create our object to send
	registerObject := &models.LoginRequestModel{
		Username: username,
		Password: hashRemainder,
		Version:  VERSION,
	}
	// Set our local variables
	pHash = hashString
	// Send our username, hash remainder.
	_, err := c.Send(
		plib.CMD_LOGIN,
		utils.MarshalResponse(registerObject),
	)
	if err != nil {
		app.Stop()
	}
}

// cmdMsgTo - Send a private encrypted message to a particular user
func (c *Client) cmdMsgTo(m *models.Message) {
	// Retry failed messages
	if inboxFailedMessageCount > 0 {
		inboxRetryFailedMessages(m.To)
	}
	// Attempt to send our message
	_, err := c.Send(plib.CMD_MSGTO, utils.MarshalResponse(&models.MsgToRequestModel{
		Message: *m,
	}))
	if err != nil {
		app.Stop()
	}
}

// ||
// Server Responses
// ||
func (c *Client) svrBlock(p []byte) {
	var blockObj models.BlockResponseModel
	if err := json.Unmarshal(p, &blockObj); err != nil {
		app.Stop()
	}
	// Set our available blocks
	blocks = blockObj.Blocks
}

func (c *Client) svrRegister(p []byte) {
	var regObj models.RegisterResponseModel
	if err := json.Unmarshal(p, &regObj); err != nil {
		app.Stop()
	}
	if !regObj.Success {
		showError(ClientError{
			Message: regObj.Message,
			Button:  "Continue",
			Continue: func() {
				pages.SwitchToPage("login")
			},
		})
		return
	}
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
	// Set variables
	c.Username = loginObj.Username // set our username
	blocks = loginObj.Blocks // set our available blocks to spend
	// Load our private key
	b, err := utils.LoadFile(c.Username)
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
	// get our contacts
	go drawContactsList()
}

func (c *Client) svrMsg(p []byte) {
	var msgObj models.MsgResponseModel
	if err := json.Unmarshal(p, &msgObj); err != nil {
		log.Debug("unable to unmarshal packet")
		return
	}
	// Mark our message as being received successfully
	msgObj.Success = true
	// Add our message to our local DB
	if _, err := dbMessageAdd(&msgObj.Message, msgObj.From); err != nil {
		panic(err)
	}
	// Make sure we are viewing the messages for the incoming message
	if inboxSelectedUsername == msgObj.From {
		// reload our message container
		go messageLoad(msgObj.From, inboxMessageContainer)
	}
}

func (c *Client) svrMsgTo(p []byte) {
	var msgObj models.MsgToResponseModel
	if err := json.Unmarshal(p, &msgObj); err != nil {
		log.Debug("unable to unmarshal packet")
		return
	}
	// Mark our request successful
	if msgObj.Success {
		if err := dbMessageSuccess(msgObj.MsgID, msgObj.To); err != nil {
			panic(err)
		}
	} else {
		if err := dbMessageFail(msgObj.MsgID, msgObj.To); err != nil {
			panic(err)
		}
		inboxFailedMessageCount++
	}
	// redraw our messages
	go messageLoad(msgObj.To, inboxMessageContainer)
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
	// Reset UI
	inboxToField.SetText("")
	app.SetFocus(userListContainer)
	go drawContactsList()
}
