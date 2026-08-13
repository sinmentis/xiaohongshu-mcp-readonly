package xiaohongshu

import "encoding/json"

type FeedsValue struct {
	Value []Feed `json:"_value"`
}

type Feed struct {
	XsecToken string   `json:"xsecToken"`
	ID        string   `json:"id"`
	ModelType string   `json:"modelType"`
	NoteCard  NoteCard `json:"noteCard"`
	Index     int      `json:"index"`
}

const modelTypeNote = "note"

// Feed lists also contain live and search cards without usable note data.
func onlyNotes(feeds []Feed) []Feed {
	notes := make([]Feed, 0, len(feeds))
	for _, f := range feeds {
		if f.ModelType == modelTypeNote {
			notes = append(notes, f)
		}
	}
	return notes
}

type NoteCard struct {
	Type         string       `json:"type"`
	DisplayTitle string       `json:"displayTitle"`
	User         User         `json:"user"`
	InteractInfo InteractInfo `json:"interactInfo"`
	Cover        Cover        `json:"cover"`
	Video        *Video       `json:"video,omitempty"` // 视频内容，可能为空
}

type User struct {
	UserID   string `json:"userId"`
	Nickname string `json:"nickname"`
	NickName string `json:"nickName"`
	Avatar   string `json:"avatar"`
}

type InteractInfo struct {
	Liked      bool   `json:"liked"`
	LikedCount string `json:"likedCount"`

	SharedCount  string `json:"sharedCount"`
	CommentCount string `json:"commentCount"`

	CollectedCount string `json:"collectedCount"`
	Collected      bool   `json:"collected"`
}

type Cover struct {
	Width      int         `json:"width"`
	Height     int         `json:"height"`
	URL        string      `json:"url"`
	FileID     string      `json:"fileId"`
	URLPre     string      `json:"urlPre"`
	URLDefault string      `json:"urlDefault"`
	InfoList   []ImageInfo `json:"infoList"`
}

type ImageInfo struct {
	ImageScene string `json:"imageScene"`
	URL        string `json:"url"`
}

type Video struct {
	Capa VideoCapability `json:"capa"`
}

type VideoCapability struct {
	Duration int `json:"duration"` // 视频时长，单位秒
}

type FeedDetailResponse struct {
	Note     FeedDetail  `json:"note"`
	Comments CommentList `json:"comments"`
}

type FeedDetail struct {
	NoteID       string            `json:"noteId"`
	XsecToken    string            `json:"xsecToken"`
	Title        string            `json:"title"`
	Desc         string            `json:"desc"`
	Type         string            `json:"type"`
	Time         int64             `json:"time"`
	IPLocation   string            `json:"ipLocation"`
	User         User              `json:"user"`
	InteractInfo InteractInfo      `json:"interactInfo"`
	ImageList    []DetailImageInfo `json:"imageList"`
	Video        *VideoDetail      `json:"video,omitempty"` // 视频笔记才有，图文笔记为 nil
}

type VideoDetail struct {
	Image VideoImage      `json:"image"`
	Capa  VideoCapability `json:"capa"`
	Media VideoMedia      `json:"media"`
	// Subtitles are decoded from mediaV2 and keyed by language.
	Subtitles map[string][]VideoSubtitle `json:"subtitles,omitempty"`
}

// UnmarshalJSON extracts subtitles, the only useful field unique to mediaV2.
func (v *VideoDetail) UnmarshalJSON(data []byte) error {
	type alias VideoDetail
	aux := struct {
		MediaV2 string `json:"mediaV2"`
		*alias
	}{alias: (*alias)(v)}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.MediaV2 == "" {
		return nil
	}

	var v2 struct {
		Video struct {
			Subtitles map[string][]VideoSubtitle `json:"subtitles"`
		} `json:"video"`
	}
	// Media URLs remain useful if optional subtitle metadata is malformed.
	if err := json.Unmarshal([]byte(aux.MediaV2), &v2); err == nil {
		v.Subtitles = v2.Video.Subtitles
	}
	return nil
}

type VideoImage struct {
	FirstFrameFileID string `json:"firstFrameFileid"`
	ThumbnailFileID  string `json:"thumbnailFileid"`
}

type VideoMedia struct {
	VideoID int64     `json:"videoId"`
	Video   VideoMeta `json:"video"`
	// Stream is keyed by codec so new codecs are preserved.
	Stream map[string][]VideoStream `json:"stream"`
}

type VideoMeta struct {
	Duration    int    `json:"duration"`
	MD5         string `json:"md5"`
	HDRType     int    `json:"hdrType"`
	DRMType     int    `json:"drmType"`
	StreamTypes []int  `json:"streamTypes"`
	BizName     int    `json:"bizName"`
	BizID       string `json:"bizId"`
}

// MasterURL is signed and temporary; BackupURLs are unsigned.
type VideoStream struct {
	MasterURL  string   `json:"masterUrl"`
	BackupURLs []string `json:"backupUrls"`

	Format      string `json:"format"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Duration    int    `json:"duration"` // 毫秒
	Size        int64  `json:"size"`     // 字节
	FPS         int    `json:"fps"`
	Rotate      int    `json:"rotate"`
	QualityType string `json:"qualityType"`
	StreamType  int    `json:"streamType"`
	StreamDesc  string `json:"streamDesc"`
	HDRType     int    `json:"hdrType"`

	VideoCodec    string `json:"videoCodec"`
	VideoBitrate  int    `json:"videoBitrate"`
	VideoDuration int    `json:"videoDuration"`
	AvgBitrate    int    `json:"avgBitrate"`

	AudioCodec    string  `json:"audioCodec"`
	AudioBitrate  int     `json:"audioBitrate"`
	AudioDuration int     `json:"audioDuration"`
	AudioChannels int     `json:"audioChannels"`
	Volume        float64 `json:"volume"`

	VMAF          float64 `json:"vmaf"`
	PSNR          float64 `json:"psnr"`
	SSIM          float64 `json:"ssim"`
	Weight        int     `json:"weight"`
	DefaultStream int     `json:"defaultStream"`
}

type VideoSubtitle struct {
	URL      string `json:"url"`
	Language string `json:"language"`
	Format   int    `json:"format"`
	Type     int    `json:"type"`
}

type DetailImageInfo struct {
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	URLDefault string `json:"urlDefault"`
	URLPre     string `json:"urlPre"`
	LivePhoto  bool   `json:"livePhoto,omitempty"`
}

type CommentList struct {
	List    []Comment `json:"list"`
	Cursor  string    `json:"cursor"`
	HasMore bool      `json:"hasMore"`
}

type Comment struct {
	ID              string    `json:"id"`
	NoteID          string    `json:"noteId"`
	Content         string    `json:"content"`
	LikeCount       string    `json:"likeCount"`
	CreateTime      int64     `json:"createTime"`
	IPLocation      string    `json:"ipLocation"`
	Liked           bool      `json:"liked"`
	UserInfo        User      `json:"userInfo"`
	SubCommentCount string    `json:"subCommentCount"`
	SubComments     []Comment `json:"subComments"`
	ShowTags        []string  `json:"showTags"`
}

type UserProfileResponse struct {
	UserBasicInfo UserBasicInfo      `json:"userBasicInfo"`
	Interactions  []UserInteractions `json:"interactions"`
	Feeds         []Feed             `json:"feeds"`
}

type UserBasicInfo struct {
	Gender     int    `json:"gender"`
	IpLocation string `json:"ipLocation"`
	Desc       string `json:"desc"`
	Imageb     string `json:"imageb"`
	Nickname   string `json:"nickname"`
	Images     string `json:"images"`
	RedId      string `json:"redId"`
}

type UserInteractions struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Count string `json:"count"`
}
