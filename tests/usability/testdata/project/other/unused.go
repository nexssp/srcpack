package other

import "strings"

type Message struct {
	Sender  string
	Subject string
}

func NormalizeMessage(message Message) Message {
	message.Sender = strings.TrimSpace(message.Sender)
	message.Subject = strings.Join(strings.Fields(message.Subject), " ")
	return message
}
