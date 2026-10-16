package zalo

import (
	"bytes"
	"encoding/json"
	"sort"

	"github.com/amrakk/zcago/model"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

const (
	contentShapeKeyLimit = 32
	contentShapeObject   = "object"
	contentShapeArray    = "array"
	contentShapeString   = "string"
	contentShapeInvalid  = "invalid"
)

func contentShape(d model.TMessage) *domain.ContentShape {
	if d.Content.Attachment == nil {
		return nil
	}
	a := d.Content.Attachment
	shape := &domain.ContentShape{
		ActionToken: knownContentAction(a.Action), TypeToken: knownContentType(a.Type),
		ActionPresent: a.Action != "", TypePresent: a.Type != "",
		UnknownKeyCount: 0, KeysTruncated: false,
		ParamShapes:        nil,
		DescriptionPresent: a.Description != "", ParamsShape: "absent", ParamsKeys: []string{},
		DescriptionMatchesTitle: a.Title != "" && a.Description == a.Title,
		ParamDescMatchesTitle:   false,
	}
	if len(a.Params) > 64<<10 {
		shape.ParamsShape = "oversized"
		return shape
	}
	params := bytes.TrimSpace([]byte(a.Params))
	if len(params) == 0 {
		return shape
	}
	if !json.Valid(params) {
		shape.ParamsShape = contentShapeInvalid
		return shape
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(params))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		shape.ParamsShape = contentShapeInvalid
		return shape
	}
	switch v := value.(type) {
	case map[string]any:
		shape.ParamsShape = contentShapeObject
		shape.ParamShapes = contentParamShapes(v)
		if description, ok := v[nativeDescriptionField].(string); ok {
			shape.ParamDescMatchesTitle = a.Title != "" && a.Title == description
		}
		for key := range v {
			if knownContentKey(key) {
				shape.ParamsKeys = append(shape.ParamsKeys, key)
			} else {
				shape.KeysTruncated = true
				shape.UnknownKeyCount++
			}
		}
		sort.Strings(shape.ParamsKeys)
		if len(shape.ParamsKeys) > contentShapeKeyLimit {
			shape.ParamsKeys = shape.ParamsKeys[:contentShapeKeyLimit]
			shape.KeysTruncated = true
		}
	case []any:
		shape.ParamsShape = contentShapeArray
	case string:
		shape.ParamsShape = contentShapeString
	default:
		shape.ParamsShape = "scalar"
	}
	return shape
}

func contentParamShapes(params map[string]any) map[string]domain.ContentParamShape {
	shapes := make(map[string]domain.ContentParamShape)
	for _, key := range []string{nativeCallIDField, nativeCallTypeField, nativeCallDurationField, nativeCallReasonField,
		nativeDescriptionField, nativePhotoWidthField, nativePhotoHeightField, "fileName", "fileSize", "totalSize", "extention",
		nativeAlbumLayoutIDField, nativeAlbumLayoutFlagField, nativeAlbumIndexField, nativeAlbumTotalField} {
		value, present := params[key]
		if !present {
			continue
		}
		description := domain.ContentParamShape{Type: "null", Empty: nil}
		empty := true
		switch typed := value.(type) {
		case string:
			description.Type, empty = contentShapeString, len(typed) == 0
		case []any:
			description.Type, empty = contentShapeArray, len(typed) == 0
		case map[string]any:
			description.Type, empty = contentShapeObject, len(typed) == 0
		case json.Number:
			description.Type = "number"
		case bool:
			description.Type = "boolean"
		}
		if description.Type != "number" && description.Type != "boolean" {
			description.Empty = &empty
		}
		shapes[key] = description
	}
	return shapes
}

// Tokens and key names are a closed diagnostic vocabulary, not a source mapping.
// A lexical filter alone could disclose private strings masquerading as keys.
func knownContentAction(value string) *string {
	switch value {
	case "sendBubbleMessage",
		nativeCallTimeAction,
		nativeMissCallAction,
		"recommened.groupcall",
		"recommened.link",
		"recommened.user":
		return &value
	default:
		return nil
	}
}

func knownContentType(value string) *string {
	switch value {
	case "call", "voice", "video", contentLink, "contact", contentImage, "file", "location":
		return &value
	default:
		return nil
	}
}

func knownContentKey(value string) bool {
	switch value {
	case nativeCallIDField,
		"fileName",
		"fileSize",
		"totalSize",
		"extention",
		"callType",
		"callTime",
		nativeCallDurationField,
		"status",
		nativeCallReasonField,
		"type",
		"mode",
		"event",
		"data",
		"ext",
		"startTime",
		"endTime",
		"isVideo",
		"call_type",
		"call_id",
		"calltime",
		nativeCallTypeField,
		"call_time",
		"call_status",
		"duration_ms",
		"photoId",
		"clientId",
		nativeDescriptionField,
		nativePhotoWidthField,
		nativePhotoHeightField,
		"toid",
		"grid",
		"rawUrl",
		"hdUrl",
		"thumbUrl",
		"oriUrl",
		"normalUrl",
		"hdSize",
		"zsource",
		"ttl",
		"jcp",
		nativeAlbumLayoutIDField,
		nativeAlbumLayoutFlagField,
		nativeAlbumIndexField,
		nativeAlbumTotalField,
		"extMsgProp",
		"mentionInfo":
		return true
	default:
		return false
	}
}
