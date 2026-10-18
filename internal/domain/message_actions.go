package domain

// SeenMetadata is private native provenance required by an explicit read receipt.
// It is absent when any required native counter was omitted or null. No raw
// message body, account credential or upstream type crosses this boundary.
type SeenMetadata struct {
	ClientMessageID string
	RecipientID     string
	MessageType     string
	Timestamp       string
	ST              int
	AT              int
	CMD             int
}
