package zalo

// Shared wire vocabulary retains the existing serialized spellings.
const (
	nativePhoto                = "chat.photo"
	nativeFile                 = "share.file"
	nativeRecommended          = "chat.recommended"
	contentImage               = "image"
	contentLink                = "link"
	metadataOnly               = "metadata_only"
	nativePhotoHeightField     = "height"
	nativePhotoWidthField      = "width"
	nativeAlbumLayoutIDField   = "groupLayoutId"
	nativeAlbumLayoutFlagField = "isGroupLayout"
	nativeAlbumIndexField      = "idInGroup"
	nativeAlbumTotalField      = "totalItemInGroup"
	contentFileNameLimit       = 256
	nativeDescriptionField     = "desc"
	nativeCallDurationField    = "duration"
	nativeCallReasonField      = "reason"
	sourceLive                 = "live"
	sourceReplay               = "replay"
	nativeCallIDField          = "callId"
	nativeCallTypeField        = "calltype"
	callActorNameLimit         = 256
	wireNull                   = "null"
	metadataUnavailable        = "unavailable"
	reactionShapeCountLimit    = 16
	nativeUnknown              = "unknown"
	nativeClientWeb            = "Web"
	nativeActionIDField        = "actionId"
	nativeClientMessageIDField = "cliMsgId"
	directionIncoming          = "incoming"
	directionOutgoing          = "outgoing"
	nativeRealMessageIDField   = "realMsgId"
	nativeUserIDField          = "userId"
)
