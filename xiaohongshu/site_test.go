package xiaohongshu

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSite(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, SetSite(SiteXiaohongshu))
	})

	require.NoError(t, SetSite(SiteRednote))
	site := Site()

	assert.Equal(t, SiteRednote, site.Name)
	assert.Equal(t, "https://www.rednote.com/explore", site.Home)
	assert.True(t, site.ForceZhCN)
	assert.True(t, site.MatchesURL("https://www.rednote.com/explore?source=web"))
	assert.False(t, site.MatchesURL("https://www.xiaohongshu.com/explore"))
	assert.Error(t, SetSite("unknown"))
}
