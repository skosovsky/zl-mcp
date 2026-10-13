package mobilebackup

// Native field tags and scalar widths retain the observed binary codec contract.
const (
	attachmentTypeTag           = 40
	attachmentCategoryIDTag     = 41
	attachmentIDTag             = 42
	attachmentExtInfoTag        = 43
	attachmentChildNumberTag    = 44
	attachmentActionTag         = 45
	attachmentParamsTag         = 46
	attachmentTitleTag          = 47
	attachmentHrefTag           = 48
	attachmentThumbTag          = 49
	attachmentDescriptionTag    = 50
	attachmentRemainsTag        = 66
	attachmentInstantDataTag    = 67
	attachmentInstantMessageTag = 68
	quoteOwnerIDTag             = 80
	quoteClientMessageIDTag     = 81
	quoteGlobalMessageIDTag     = 82
	quoteMessageTypeTag         = 83
	quoteTimestampTag           = 84
	quoteTTLTag                 = 85
	quoteMessageTag             = 86
	quoteAttachmentTag          = 87
	quoteFromDTag               = 88
	quoteStatusTag              = 90
	mentionTypeTag              = 100
	mentionUIDTag               = 101
	mentionPositionTag          = 102
	mentionLengthTag            = 103
	binNetAttachmentTag         = 6
	binNetQuoteTag              = 7
	binNetMentionTag            = 8
	nativeInt32Bytes            = 4
	nativeInt64Bytes            = 8
	quoteScalarFieldCount       = 6
)
