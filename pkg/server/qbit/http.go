package qbit

import (
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/internal/utils"
	"github.com/sirrobot01/decypharr/pkg/arr"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

func parseSeedingPolicy(r *http.Request) (*storage.SeedingPolicy, error) {
	var policy storage.SeedingPolicy

	if value := strings.TrimSpace(r.FormValue("ratioLimit")); value != "" {
		ratio, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid ratioLimit value %q", value)
		}
		switch {
		case ratio >= 0:
			policy.StopOnRatio = &ratio
		case ratio == -1 || ratio == -2:
			// qBittorrent uses -1 for no limit and -2 for global limit.
		default:
			return nil, fmt.Errorf("invalid ratioLimit value %q", value)
		}
	}

	if value := strings.TrimSpace(r.FormValue("seedingTimeLimit")); value != "" {
		minutes, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("invalid seedingTimeLimit value %q", value)
		}
		switch {
		case minutes >= 0:
			policy.StopAfterMinutes = &minutes
		case minutes == -1 || minutes == -2:
			// qBittorrent uses -1 for no limit and -2 for global limit.
		default:
			return nil, fmt.Errorf("invalid seedingTimeLimit value %q", value)
		}
	}

	if policy.StopOnRatio == nil && policy.StopAfterMinutes == nil {
		return nil, nil
	}

	return &policy, nil
}

func sortQBitEntriesByAddedOnDesc(entries []*storage.Entry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].AddedOn.After(entries[j].AddedOn)
	})
}

func cloneQBitSeedingPolicy(policy *storage.SeedingPolicy) *storage.SeedingPolicy {
	if policy == nil {
		return nil
	}

	cloned := &storage.SeedingPolicy{
		LastStopError: policy.LastStopError,
	}
	if policy.StopOnRatio != nil {
		ratio := *policy.StopOnRatio
		cloned.StopOnRatio = &ratio
	}
	if policy.StopAfterMinutes != nil {
		minutes := *policy.StopAfterMinutes
		cloned.StopAfterMinutes = &minutes
	}
	if policy.StopRequestedAt != nil {
		requestedAt := *policy.StopRequestedAt
		cloned.StopRequestedAt = &requestedAt
	}
	if policy.StopCompletedAt != nil {
		completedAt := *policy.StopCompletedAt
		cloned.StopCompletedAt = &completedAt
	}
	return cloned
}

func (q *QBit) handleLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg := config.Get()
	username := r.FormValue("username")
	password := r.FormValue("password")
	a, err := q.authenticate(getCategory(ctx), username, password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if cfg.UseAuth {
		cookie := &http.Cookie{
			Name:     "sid",
			Value:    createSID(a.Host, a.Token),
			Path:     "/",
			SameSite: http.SameSiteNoneMode,
		}
		http.SetCookie(w, cookie)
	}
	_, _ = w.Write([]byte("Ok."))
}

func (q *QBit) handleVersion(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte("v4.3.2"))
}

func (q *QBit) handleWebAPIVersion(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte("2.7"))
}

func (q *QBit) handlePreferences(w http.ResponseWriter, r *http.Request) {
	preferences := getAppPreferences()

	preferences.SavePath = q.downloadFolder
	preferences.TempPath = filepath.Join(q.downloadFolder, "temp")

	utils.JSONResponse(w, preferences, http.StatusOK)
}

func (q *QBit) handleBuildInfo(w http.ResponseWriter, r *http.Request) {
	res := BuildInfo{
		Bitness:    64,
		Boost:      "1.75.0",
		Libtorrent: "1.2.11.0",
		Openssl:    "1.1.1i",
		Qt:         "5.15.2",
		Zlib:       "1.2.11",
	}
	utils.JSONResponse(w, res, http.StatusOK)
}

func (q *QBit) handleShutdown(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (q *QBit) handleTorrentsInfo(w http.ResponseWriter, r *http.Request) {
	//log all url params
	ctx := r.Context()
	category := getCategory(ctx)
	state := strings.Trim(r.URL.Query().Get("filter"), "")
	hashes := getHashes(ctx)

	// Convert hashes to filter function
	torrents, err := q.manager.ListVisibleEntries(category, config.ProtocolTorrent, storage.TorrentState(state), hashes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sortQBitEntriesByAddedOnDesc(torrents)
	qbitTorrents := make([]Torrent, len(torrents))
	for i, t := range torrents {
		qbitTorrents[i] = convertToQBitTorrentTorrent(t)
	}
	utils.JSONResponse(w, qbitTorrents, http.StatusOK)
}

func (q *QBit) handleTorrentsAdd(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse form based on content type
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			q.logger.Error().Err(err).Msgf("Error parsing multipart form")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		if err := r.ParseForm(); err != nil {
			q.logger.Error().Err(err).Msgf("Error parsing form")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		http.Error(w, "Invalid content type", http.StatusBadRequest)
		return
	}

	cfg := config.Get()
	action := cfg.DefaultDownloadAction
	if strings.ToLower(r.FormValue("sequentialDownload")) == "true" {
		action = config.DownloadActionDownload
	}

	rmTrackerUrls := strings.ToLower(r.FormValue("firstLastPiecePrio")) == "true"

	// Check config setting - if always remove tracker URLs is enabled, force it to true
	if q.alwaysRemoveTrackerURLS {
		rmTrackerUrls = true
	}

	debridName := r.FormValue("debrid")
	category := r.FormValue("category")
	seedingPolicy, err := parseSeedingPolicy(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	policyLog := q.logger.Debug().
		Str("debrid", debridName).
		Str("category", category).
		Str("ratio_limit_raw", strings.TrimSpace(r.FormValue("ratioLimit"))).
		Str("seeding_time_limit_raw", strings.TrimSpace(r.FormValue("seedingTimeLimit")))
	if seedingPolicy != nil {
		if seedingPolicy.StopOnRatio != nil {
			policyLog = policyLog.Float64("ratio_limit", *seedingPolicy.StopOnRatio)
		}
		if seedingPolicy.StopAfterMinutes != nil {
			policyLog = policyLog.Int("seeding_time_limit_minutes", *seedingPolicy.StopAfterMinutes)
		}
		policyLog.Msg("Parsed qBittorrent seeding policy from add request")
	} else {
		policyLog.Msg("No per-torrent seeding policy found in qBittorrent add request")
	}

	_arr := getArrFromContext(ctx)
	if _arr == nil {
		// Arr is not in context
		_arr = arr.New(category, "", "", false, false, nil, "", "")
	}
	atleastOne := false

	// Handle magnet URLs
	if urls := r.FormValue("urls"); urls != "" {
		var urlList []string
		for _, u := range strings.Split(urls, "\n") {
			urlList = append(urlList, strings.TrimSpace(u))
		}
		for _, url := range urlList {
			if err := q.addMagnet(ctx, url, _arr, debridName, action, cfg.CallbackURL, rmTrackerUrls, cfg.SkipMultiSeason, seedingPolicy); err != nil {
				q.logger.Debug().Msgf("Error adding magnet: %s", err.Error())
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			atleastOne = true
		}
	}

	// Handle torrent files
	if r.MultipartForm != nil && r.MultipartForm.File != nil {
		if files := r.MultipartForm.File["torrents"]; len(files) > 0 {
			for _, fileHeader := range files {
				if err := q.addTorrent(ctx, fileHeader, _arr, debridName, action, cfg.CallbackURL, rmTrackerUrls, cfg.SkipMultiSeason, seedingPolicy); err != nil {
					q.logger.Debug().Err(err).Msgf("Error adding torrent")
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				atleastOne = true
			}
		}
	}

	if !atleastOne {
		http.Error(w, "No valid URLs or torrents provided", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (q *QBit) handleTorrentsDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hashes := getHashes(ctx)

	if len(hashes) == 0 {
		http.Error(w, "No hashes provided", http.StatusBadRequest)
		return
	}
	for _, hash := range hashes {
		err := q.manager.Queue().Delete(hash, nil)
		if err != nil && !strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		exists, existsErr := q.manager.EntryExists(hash)
		if existsErr != nil {
			http.Error(w, existsErr.Error(), http.StatusInternalServerError)
			return
		}
		if exists {
			if err := q.manager.DeleteEntry(hash, false); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (q *QBit) handleTorrentsSetShareLimits(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	hashes := getHashes(ctx)
	if len(hashes) == 0 {
		http.Error(w, "No hashes provided", http.StatusBadRequest)
		return
	}

	inactiveRaw := strings.TrimSpace(r.FormValue("inactiveSeedingTimeLimit"))
	policy, err := parseSeedingPolicy(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(hashes) == 1 && strings.EqualFold(hashes[0], "all") {
		entries, err := q.manager.ListVisibleEntries(getCategory(ctx), config.ProtocolTorrent, "", nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		hashes = hashes[:0]
		for _, entry := range entries {
			hashes = append(hashes, entry.InfoHash)
		}
	}

	logEvent := q.logger.Debug().
		Strs("hashes", hashes).
		Str("ratio_limit_raw", strings.TrimSpace(r.FormValue("ratioLimit"))).
		Str("seeding_time_limit_raw", strings.TrimSpace(r.FormValue("seedingTimeLimit"))).
		Str("inactive_seeding_time_limit_raw", inactiveRaw)
	if policy != nil {
		if policy.StopOnRatio != nil {
			logEvent = logEvent.Float64("ratio_limit", *policy.StopOnRatio)
		}
		if policy.StopAfterMinutes != nil {
			logEvent = logEvent.Int("seeding_time_limit_minutes", *policy.StopAfterMinutes)
		}
		logEvent.Msg("Received qBittorrent setShareLimits request")
	} else {
		logEvent.Msg("Received qBittorrent setShareLimits request clearing per-torrent share limits")
	}

	updatedAt := time.Now()
	updatedAny := false

	for _, hash := range hashes {
		if hash == "" {
			continue
		}

		if torrent, err := q.manager.Queue().GetTorrent(hash); err == nil && torrent != nil {
			torrent.SeedingPolicy = cloneQBitSeedingPolicy(policy)
			torrent.UpdatedAt = updatedAt
			if err := q.manager.Queue().Update(torrent); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			updatedAny = true
		}

		if torrent, err := q.manager.GetEntry(hash); err == nil && torrent != nil {
			torrent.SeedingPolicy = cloneQBitSeedingPolicy(policy)
			torrent.UpdatedAt = updatedAt
			if err := q.manager.AddOrUpdate(torrent, nil); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			updatedAny = true
		}
	}

	if !updatedAny {
		http.Error(w, "Torrent not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (q *QBit) handleTorrentsPause(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hashes := getHashes(ctx)
	for _, hash := range hashes {
		torrent, err := q.manager.Queue().GetTorrent(hash)
		if err != nil {
			continue
		}
		go q.PauseTorrent(torrent)
	}

	w.WriteHeader(http.StatusOK)
}

func (q *QBit) handleTorrentsResume(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hashes := getHashes(ctx)
	for _, hash := range hashes {
		torrent, err := q.manager.Queue().GetTorrent(hash)
		if err != nil {
			continue
		}
		go q.ResumeTorrent(torrent)
	}

	w.WriteHeader(http.StatusOK)
}

func (q *QBit) handleTorrentRecheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hashes := getHashes(ctx)
	for _, hash := range hashes {
		torrent, err := q.manager.Queue().GetTorrent(hash)
		if err != nil {
			continue
		}
		go q.RefreshTorrent(torrent)
	}

	w.WriteHeader(http.StatusOK)
}

func (q *QBit) handleCategories(w http.ResponseWriter, r *http.Request) {
	var categories = map[string]TorrentCategory{}
	for _, cat := range q.categories {
		path := filepath.Join(q.downloadFolder, cat)
		categories[cat] = TorrentCategory{
			Name:     cat,
			SavePath: path,
		}
	}
	utils.JSONResponse(w, categories, http.StatusOK)
}

func (q *QBit) handleCreateCategory(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	name := r.Form.Get("category")
	if name == "" {
		http.Error(w, "No name provided", http.StatusBadRequest)
		return
	}

	q.categories = append(q.categories, name)

	utils.JSONResponse(w, nil, http.StatusOK)
}

func (q *QBit) handleTorrentProperties(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("hash")
	torrent, err := q.manager.Queue().GetTorrent(hash)
	if err != nil {
		http.Error(w, "Entry not found", http.StatusNotFound)
		return
	}

	properties := q.GetTorrentProperties(torrent)
	utils.JSONResponse(w, properties, http.StatusOK)
}

func (q *QBit) handleTorrentFiles(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("hash")
	torrent, err := q.manager.Queue().GetTorrent(hash)
	if err != nil {
		http.Error(w, "Entry not found", http.StatusNotFound)
		return
	}
	utils.JSONResponse(w, getTorrentFiles(torrent), http.StatusOK)
}

func (q *QBit) handleSetCategory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	category := getCategory(ctx)
	hashes := getHashes(ctx)
	var filterFunc func(t *storage.Entry) bool

	hashSet := make(map[string]bool)
	if len(hashes) > 0 {
		for _, h := range hashes {
			hashSet[h] = true
		}

	}

	updateFunc := func(t *storage.Entry) bool {
		if t.Category != category {
			t.Category = category
			return true
		}
		return false
	}

	if err := q.manager.Queue().UpdateWhere(filterFunc, updateFunc); err != nil {
		q.logger.Warn().Err(err).Msgf("Error adding torrent")
		http.Error(w, "Failed to update torrents", http.StatusInternalServerError)
		return
	}
	utils.JSONResponse(w, nil, http.StatusOK)
}

func (q *QBit) handleAddTorrentTags(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	hashes := getHashes(ctx)
	tags := strings.Split(r.FormValue("tags"), ",")
	for i, tag := range tags {
		tags[i] = strings.TrimSpace(tag)
	}
	torrents := q.manager.Queue().ListFilter("", config.ProtocolTorrent, "", hashes, "", false)
	for _, t := range torrents {
		q.setTorrentTags(t, tags)
	}
	utils.JSONResponse(w, nil, http.StatusOK)
}

func (q *QBit) handleRemoveTorrentTags(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	hashes := getHashes(ctx)
	tags := strings.Split(r.FormValue("tags"), ",")
	for i, tag := range tags {
		tags[i] = strings.TrimSpace(tag)
	}
	torrents := q.manager.Queue().ListFilter("", config.ProtocolTorrent, "", hashes, "", false)
	for _, torrent := range torrents {
		q.removeTorrentTags(torrent, tags)

	}
	utils.JSONResponse(w, nil, http.StatusOK)
}

func (q *QBit) handleGetTags(w http.ResponseWriter, r *http.Request) {
	utils.JSONResponse(w, q.Tags, http.StatusOK)
}

func (q *QBit) handleCreateTags(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}
	tags := strings.Split(r.FormValue("tags"), ",")
	for i, tag := range tags {
		tags[i] = strings.TrimSpace(tag)
	}
	q.addTags(tags)
	utils.JSONResponse(w, nil, http.StatusOK)
}
