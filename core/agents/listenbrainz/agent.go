package listenbrainz

import (
	"context"
	"net/http"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/agents"
	"github.com/navidrome/navidrome/core/scrobbler"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/utils"
)

const (
	listenBrainzAgentName = "listenbrainz"
	sessionKeyProperty    = "ListenBrainzSessionKey"
)

type listenBrainzAgent struct {
	ds          model.DataStore
	sessionKeys *agents.SessionKeys
	client      *Client
}

func listenBrainzConstructor(ds model.DataStore) *listenBrainzAgent {
	l := &listenBrainzAgent{
		ds:          ds,
		sessionKeys: &agents.SessionKeys{DataStore: ds, KeyName: sessionKeyProperty},
	}
	hc := &http.Client{
		Timeout: consts.DefaultHttpClientTimeOut,
	}
	chc := utils.NewCachedHTTPClient(hc, consts.DefaultHttpClientTimeOut)
	l.client = NewClient(chc)
	return l
}

func (l *listenBrainzAgent) AgentName() string {
	return listenBrainzAgentName
}

func (l *listenBrainzAgent) formatListen(track *model.MediaFile) listenInfo {
	li := listenInfo{
		TrackMetadata: trackMetadata{
			ArtistName:  track.Artist,
			TrackName:   track.Title,
			ReleaseName: track.Album,
			AdditionalInfo: additionalInfo{
				TrackNumber:  track.TrackNumber,
				ArtistMbzIDs: []string{track.MbzArtistID},
				TrackMbzID:   track.MbzTrackID,
				ReleaseMbID:  track.MbzAlbumID,
			},
		},
	}
	return li
}

func (l *listenBrainzAgent) NowPlaying(ctx context.Context, userId string, track *model.MediaFile) error {
	sk, err := l.sessionKeys.Get(ctx, userId)
	if err != nil || sk == "" {
		return scrobbler.ErrNotAuthorized
	}

	li := l.formatListen(track)
	err = l.client.UpdateNowPlaying(ctx, sk, li)
	if err != nil {
		log.Warn(ctx, "ListenBrainz UpdateNowPlaying returned error", "track", track.Title, err)
		return scrobbler.ErrUnrecoverable
	}
	return nil
}

func (l *listenBrainzAgent) Scrobble(ctx context.Context, userId string, s scrobbler.Scrobble) error {
	sk, err := l.sessionKeys.Get(ctx, userId)
	if err != nil || sk == "" {
		return scrobbler.ErrNotAuthorized
	}

	li := l.formatListen(&s.MediaFile)
	li.ListenedAt = int(s.TimeStamp.Unix())
	err = l.client.Scrobble(ctx, sk, li)

	if err == nil {
		return nil
	}
	lbErr, isListenBrainzError := err.(*listenBrainzError)
	if !isListenBrainzError {
		log.Warn(ctx, "ListenBrainz Scrobble returned HTTP error", "track", s.Title, err)
		return scrobbler.ErrRetryLater
	}
	if lbErr.Code == 500 || lbErr.Code == 503 {
		return scrobbler.ErrRetryLater
	}
	return scrobbler.ErrUnrecoverable
}

func (l *listenBrainzAgent) IsAuthorized(ctx context.Context, userId string) bool {
	sk, err := l.sessionKeys.Get(ctx, userId)
	return err == nil && sk != ""
}

func init() {
	conf.AddHook(func() {
		if conf.Server.DevListenBrainzEnabled {
			scrobbler.Register(listenBrainzAgentName, func(ds model.DataStore) scrobbler.Scrobbler {
				return listenBrainzConstructor(ds)
			})
		}
	})
}
