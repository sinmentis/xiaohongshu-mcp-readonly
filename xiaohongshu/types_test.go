package xiaohongshu

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Real search results mix note, live_v2, and hot_query cards.
func TestOnlyNotes(t *testing.T) {
	t.Run("滤掉直播卡片与搜索热词", func(t *testing.T) {
		feeds := []Feed{
			{ID: "1", ModelType: "note", NoteCard: NoteCard{Type: "normal", DisplayTitle: "山顶露营"}},
			{ID: "2", ModelType: "live_v2"},
			{ID: "3", ModelType: "hot_query"},
			{ID: "4", ModelType: "note", NoteCard: NoteCard{Type: "normal", DisplayTitle: "露营日记"}},
		}

		got := onlyNotes(feeds)

		assert.Len(t, got, 2)
		assert.Equal(t, "1", got[0].ID)
		assert.Equal(t, "4", got[1].ID)
	})

	t.Run("视频笔记不被误伤", func(t *testing.T) {
		feeds := []Feed{
			{ID: "1", ModelType: "note", NoteCard: NoteCard{Type: "video"}},
			{ID: "2", ModelType: "note", NoteCard: NoteCard{Type: "normal"}},
		}

		assert.Len(t, onlyNotes(feeds), 2, "视频与图文的 modelType 同为 note，都要保留")
	})

	t.Run("无标题的笔记要保留", func(t *testing.T) {
		// 平台允许笔记没有标题，这类条目 displayTitle 为空但确实是笔记，
		// 不能跟着非笔记条目一起滤掉
		feeds := []Feed{{ID: "1", ModelType: "note", NoteCard: NoteCard{Type: "normal"}}}

		assert.Len(t, onlyNotes(feeds), 1)
	})

	t.Run("全是非笔记时返回空而不是 nil", func(t *testing.T) {
		got := onlyNotes([]Feed{{ModelType: "hot_query"}})

		assert.NotNil(t, got, "返回空切片，避免调用方拿到 nil 再判空")
		assert.Empty(t, got)
	})
}

// Upstream sends videoId as a JSON number larger than 2^53.
func TestVideoIDJSON(t *testing.T) {
	t.Run("从数字解码并以字符串编码", func(t *testing.T) {
		var media VideoMedia

		require.NoError(t, json.Unmarshal([]byte(`{"videoId":138040748843676370}`), &media))
		assert.Equal(t, VideoID(138040748843676370), media.VideoID)

		encoded, err := json.Marshal(media.VideoID)
		require.NoError(t, err)
		assert.JSONEq(t, `"138040748843676370"`, string(encoded))
	})

	t.Run("字符串与 null 也能解码", func(t *testing.T) {
		var fromString, fromNull VideoID

		require.NoError(t, json.Unmarshal([]byte(`"42"`), &fromString))
		assert.Equal(t, VideoID(42), fromString)

		require.NoError(t, json.Unmarshal([]byte(`null`), &fromNull))
		assert.Equal(t, VideoID(0), fromNull)
	})

	t.Run("整条视频笔记编码后不含裸大整数", func(t *testing.T) {
		detail := FeedDetail{
			NoteID: "6a6b54680000000005028fef",
			Video:  &VideoDetail{Media: VideoMedia{VideoID: 138040748843676370}},
		}

		encoded, err := json.Marshal(detail)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), `:138040748843676370`)
		assert.Contains(t, string(encoded), `"videoId":"138040748843676370"`)
	})
}
