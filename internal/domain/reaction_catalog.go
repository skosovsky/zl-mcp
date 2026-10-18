package domain

// ReactionOption is a stable agent-facing choice, without protocol type/source.
type ReactionOption struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Display string `json:"display"`
}

// ReactionCatalog returns a fresh finite catalogue of upstream-defined encodings.
// Native acceptance of every icon is not implied by catalogue membership.
func ReactionCatalog() []ReactionOption {
	return []ReactionOption{
		{Code: "heart", Name: "Heart", Display: "❤️"},
		{Code: "like", Name: "Like", Display: "👍"},
		{Code: "haha", Name: "Haha", Display: "Haha"},
		{Code: "wow", Name: "Wow", Display: "Wow"},
		{Code: "cry", Name: "Cry", Display: "Cry"},
		{Code: "angry", Name: "Angry", Display: "Angry"},
		{Code: "kiss", Name: "Kiss", Display: "Kiss"},
		{Code: "tears_of_joy", Name: "Tears Of Joy", Display: "Tears Of Joy"},
		{Code: "shit", Name: "Shit", Display: "Shit"},
		{Code: "rose", Name: "Rose", Display: "Rose"},
		{Code: "broken_heart", Name: "Broken Heart", Display: "Broken Heart"},
		{Code: "dislike", Name: "Dislike", Display: "Dislike"},
		{Code: "love", Name: "Love", Display: "Love"},
		{Code: "confused", Name: "Confused", Display: "Confused"},
		{Code: "wink", Name: "Wink", Display: "Wink"},
		{Code: "fade", Name: "Fade", Display: "Fade"},
		{Code: "sun", Name: "Sun", Display: "Sun"},
		{Code: "birthday", Name: "Birthday", Display: "Birthday"},
		{Code: "bomb", Name: "Bomb", Display: "Bomb"},
		{Code: "ok", Name: "OK", Display: "OK"},
		{Code: "peace", Name: "Peace", Display: "Peace"},
		{Code: "thanks", Name: "Thanks", Display: "Thanks"},
		{Code: "punch", Name: "Punch", Display: "Punch"},
		{Code: "share", Name: "Share", Display: "Share"},
		{Code: "pray", Name: "Pray", Display: "Pray"},
		{Code: "no", Name: "No", Display: "No"},
		{Code: "bad", Name: "Bad", Display: "Bad"},
		{Code: "love_you", Name: "Love You", Display: "Love You"},
		{Code: "sad", Name: "Sad", Display: "Sad"},
		{Code: "very_sad", Name: "Very Sad", Display: "Very Sad"},
		{Code: "cool", Name: "Cool", Display: "Cool"},
		{Code: "nerd", Name: "Nerd", Display: "Nerd"},
		{Code: "big_smile", Name: "Big Smile", Display: "Big Smile"},
		{Code: "sunglasses", Name: "Sunglasses", Display: "Sunglasses"},
		{Code: "neutral", Name: "Neutral", Display: "Neutral"},
		{Code: "sad_face", Name: "Sad Face", Display: "Sad Face"},
		{Code: "bye", Name: "Bye", Display: "Bye"},
		{Code: "sleepy", Name: "Sleepy", Display: "Sleepy"},
		{Code: "wipe", Name: "Wipe", Display: "Wipe"},
		{Code: "dig", Name: "Dig", Display: "Dig"},
		{Code: "anguish", Name: "Anguish", Display: "Anguish"},
		{Code: "handclap", Name: "Handclap", Display: "Handclap"},
		{Code: "angry_face", Name: "Angry Face", Display: "Angry Face"},
		{Code: "f_chair", Name: "F Chair", Display: "F Chair"},
		{Code: "l_chair", Name: "L Chair", Display: "L Chair"},
		{Code: "r_chair", Name: "R Chair", Display: "R Chair"},
		{Code: "silent", Name: "Silent", Display: "Silent"},
		{Code: "surprise", Name: "Surprise", Display: "Surprise"},
		{Code: "embarrassed", Name: "Embarrassed", Display: "Embarrassed"},
		{Code: "afraid", Name: "Afraid", Display: "Afraid"},
		{Code: "sad_2", Name: "Sad 2", Display: "Sad 2"},
		{Code: "big_laugh", Name: "Big Laugh", Display: "Big Laugh"},
		{Code: "rich", Name: "Rich", Display: "Rich"},
		{Code: "beer", Name: "Beer", Display: "Beer"},
	}
}
