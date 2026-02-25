package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"awesomeProject/internal/btsync"
	"awesomeProject/internal/gui/config"
	"awesomeProject/internal/midi"
	"awesomeProject/internal/obsws"

	"github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/requests/scenes"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type BluetoothStatus = btsync.Status

type App struct {
	ctx context.Context
	cfg *config.Config

	// MIDI runtime
	midiCancel context.CancelFunc
	midiDrv    midi.Input

	// OBS connection cache
	cacheMu      sync.Mutex
	cacheClients map[string]*goobs.Client // key: addr+"\x00"+password

	// Bluetooth sync manager
	bt *btsync.Manager
}

func NewApp() *App {
	cfg, _ := config.Load()
	if cfg == nil {
		cfg = config.Default()
	}
	allDisabled := true
	for i := range cfg.Connections {
		if cfg.Connections[i].Enabled {
			allDisabled = false
			break
		}
	}
	if len(cfg.Connections) > 0 && allDisabled {
		for i := range cfg.Connections {
			cfg.Connections[i].Enabled = true
		}
	}

	a := &App{cfg: cfg, cacheClients: map[string]*goobs.Client{}}
	a.bt = btsync.NewManager(btsync.NewNativeTransport(), btsync.ManagerOptions{
		ApplyScene: func(scene string, source btsync.Source) error {
			return a.applySceneToEnabledConnections(scene)
		},
		SceneExists: func(scene string) (bool, error) {
			return a.sceneExistsOnEnabledConnections(scene)
		},
		PersistTrustedPeers: func(peers []btsync.TrustedPeer) error {
			a.cfg.Bluetooth.TrustedPeers = toConfigTrustedPeers(peers)
			return config.Save(a.cfg)
		},
		Logf: func(level, msg string) {
			_ = a.emitLog(level, "[BT] "+msg)
		},
	})
	a.bt.SetConfig(btConfigFromGUI(a.cfg.Bluetooth))
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	_ = a.emitLog("info", "GUI 起動")
}

func (a *App) shutdown(ctx context.Context) {
	_ = a.BtStop()
	_ = a.MidiStop()
	a.cacheMu.Lock()
	for _, c := range a.cacheClients {
		_ = c.Disconnect()
	}
	a.cacheClients = map[string]*goobs.Client{}
	a.cacheMu.Unlock()
}

// --- 設定API ---

func (a *App) GetConfig() (*config.Config, error) {
	if a.cfg == nil {
		return config.Default(), nil
	}
	return a.cfg, nil
}

func (a *App) SaveConfig(c *config.Config) error {
	if c == nil {
		return errors.New("config is nil")
	}
	normalizeUniqueConnectionNames(c)
	a.cfg = c
	if err := config.Save(c); err != nil {
		return err
	}
	a.bt.SetConfig(btConfigFromGUI(a.cfg.Bluetooth))
	return a.emitLog("info", "設定を保存しました")
}

func normalizeUniqueConnectionNames(c *config.Config) {
	if c == nil || len(c.Connections) == 0 {
		return
	}
	used := map[string]struct{}{}
	for i := range c.Connections {
		name := strings.TrimSpace(c.Connections[i].Name)
		if name == "" {
			name = fmt.Sprintf("OBS %d", i+1)
		}
		base := name
		try := base
		suffix := 2
		for {
			key := strings.ToLower(strings.TrimSpace(try))
			if _, ok := used[key]; !ok {
				c.Connections[i].Name = try
				used[key] = struct{}{}
				break
			}
			try = fmt.Sprintf("%s (%d)", base, suffix)
			suffix++
		}
	}
}

func (a *App) TestConnections() (map[string]string, error) {
	if len(a.cfg.Connections) == 0 {
		return nil, errors.New("接続先がありません")
	}
	out := map[string]string{}
	for _, c := range a.cfg.Connections {
		if !c.Enabled {
			out[c.Name] = "SKIP: disabled"
			continue
		}
		addr := obsws.NormalizeObsAddr(c.Addr)
		cli, err := openObs(addr, strings.TrimSpace(c.Password))
		if err != nil {
			out[c.Name] = fmt.Sprintf("NG: %v", err)
			continue
		}
		if _, err := cli.Scenes.GetSceneList(nil); err != nil {
			out[c.Name] = fmt.Sprintf("NG: %v", err)
		} else {
			out[c.Name] = "OK"
		}
		_ = cli.Disconnect()
	}
	return out, nil
}

func (a *App) ListScenes() ([]string, error) {
	if len(a.cfg.Connections) == 0 {
		return nil, errors.New("接続先がありません")
	}
	var inter map[string]struct{}
	first := true
	for _, c := range a.cfg.Connections {
		if !c.Enabled {
			continue
		}
		addr := obsws.NormalizeObsAddr(c.Addr)
		cli, err := a.getClientCached(addr, strings.TrimSpace(c.Password))
		if err != nil {
			return nil, fmt.Errorf("%s への接続に失敗: %w", c.Name, err)
		}
		lst, err := cli.Scenes.GetSceneList(nil)
		if err != nil {
			return nil, fmt.Errorf("%s のシーン一覧取得に失敗: %w", c.Name, err)
		}
		set := map[string]struct{}{}
		for _, s := range lst.Scenes {
			set[s.SceneName] = struct{}{}
		}
		if first {
			inter = set
			first = false
		} else {
			for k := range inter {
				if _, ok := set[k]; !ok {
					delete(inter, k)
				}
			}
		}
	}
	if first {
		return nil, errors.New("有効な接続がありません")
	}
	out := make([]string, 0, len(inter))
	for k := range inter {
		out = append(out, k)
	}
	if len(out) > 1 {
		sortStrings(out)
	}
	return out, nil
}

func (a *App) ListScenesFor(connectionName string) ([]string, error) {
	if strings.TrimSpace(connectionName) == "" {
		return nil, errors.New("接続名を指定してください")
	}
	for _, c := range a.cfg.Connections {
		if !c.Enabled {
			continue
		}
		if c.Name == connectionName {
			addr := obsws.NormalizeObsAddr(c.Addr)
			cli, err := a.getClientCached(addr, strings.TrimSpace(c.Password))
			if err != nil {
				return nil, fmt.Errorf("%s への接続に失敗: %w", c.Name, err)
			}
			lst, err := cli.Scenes.GetSceneList(nil)
			if err != nil {
				return nil, fmt.Errorf("%s のシーン一覧取得に失敗: %w", c.Name, err)
			}
			names := make([]string, 0, len(lst.Scenes))
			for _, s := range lst.Scenes {
				names = append(names, s.SceneName)
			}
			if len(names) > 1 {
				sortStrings(names)
			}
			return names, nil
		}
	}
	return nil, fmt.Errorf("接続が見つかりません: %s", connectionName)
}

func (a *App) TriggerScene(scene string) error {
	scene = strings.TrimSpace(scene)
	if scene == "" {
		return errors.New("シーン名が空です")
	}

	go func() {
		if err := a.dispatchScene(scene, btsync.SourceGUI); err != nil {
			_ = a.emitLog("error", fmt.Sprintf("切替失敗: %v", err))
		}
	}()
	return nil
}

func (a *App) ImportFromDir(connectionName, dir string, loop bool, activate bool, transition string, monitoring string, debug bool) error {
	var target *config.Connection
	for i := range a.cfg.Connections {
		if a.cfg.Connections[i].Name == connectionName && a.cfg.Connections[i].Enabled {
			target = &a.cfg.Connections[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("接続が見つかりません: %s", connectionName)
	}
	if strings.TrimSpace(dir) == "" {
		return errors.New("ディレクトリを指定してください")
	}
	abs := dir
	if !filepath.IsAbs(dir) {
		if wd, err := os.Getwd(); err == nil {
			abs = filepath.Join(wd, dir)
		}
	}
	opts := obsws.ImportOptions{
		Addr:       target.Addr,
		Password:   strings.TrimSpace(target.Password),
		Dir:        abs,
		Loop:       loop,
		Activate:   activate,
		Transition: transition,
		Monitoring: monitoring,
		Debug:      debug,
	}
	go func() {
		_ = a.emitLog("info", fmt.Sprintf("インポート開始: %s -> %s", connectionName, abs))
		if err := obsws.ImportScenes(opts); err != nil {
			_ = a.emitLog("error", fmt.Sprintf("インポート失敗: %v", err))
		} else {
			_ = a.emitLog("info", "インポート完了")
		}
	}()
	return nil
}

func (a *App) emitLog(level, msg string) error {
	log.Printf("[%s] %s", level, msg)
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "log", map[string]any{"level": level, "msg": msg, "time": time.Now().Format(time.RFC3339)})
	}
	return nil
}

func sortStrings(s []string) {
	n := len(s)
	for i := 0; i < n; i++ {
		for j := 0; j < n-1-i; j++ {
			if strings.ToLower(s[j]) > strings.ToLower(s[j+1]) {
				s[j], s[j+1] = s[j+1], s[j]
			}
		}
	}
}

func (a *App) OpenDirectoryDialog(defaultDir string, title string) (string, error) {
	if a.ctx == nil {
		return "", errors.New("no context")
	}
	opts := runtime.OpenDialogOptions{
		Title:                      title,
		DefaultDirectory:           defaultDir,
		CanCreateDirectories:       true,
		ShowHiddenFiles:            false,
		TreatPackagesAsDirectories: true,
	}
	return runtime.OpenDirectoryDialog(a.ctx, opts)
}

func openObs(addr, password string) (*goobs.Client, error) {
	if strings.TrimSpace(password) == "" {
		return goobs.New(addr)
	}
	return goobs.New(addr, goobs.WithPassword(password))
}

func (a *App) OpenExternalURL(url string) error {
	if a.ctx == nil {
		return errors.New("no context")
	}
	runtime.BrowserOpenURL(a.ctx, url)
	return nil
}

// --- Cached OBS connections ---

func (a *App) getClientCached(addr, pw string) (*goobs.Client, error) {
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	key := addr + "\x00" + strings.TrimSpace(pw)
	if c, ok := a.cacheClients[key]; ok {
		return c, nil
	}
	var c *goobs.Client
	var err error
	if strings.TrimSpace(pw) == "" {
		c, err = goobs.New(addr)
	} else {
		c, err = goobs.New(addr, goobs.WithPassword(pw))
	}
	if err != nil {
		return nil, err
	}
	a.cacheClients[key] = c
	return c, nil
}

func (a *App) setSceneCachedPairs(pairs []struct{ addr, pw string }, scene string) error {
	for _, p := range pairs {
		addr := obsws.NormalizeObsAddr(strings.TrimSpace(p.addr))
		if addr == "" {
			continue
		}
		cli, err := a.getClientCached(addr, p.pw)
		if err != nil {
			return fmt.Errorf("%s 接続失敗: %w", addr, err)
		}
		_, err = cli.Scenes.SetCurrentProgramScene((&scenes.SetCurrentProgramSceneParams{}).WithSceneName(scene))
		if err != nil {
			key := addr + "\x00" + strings.TrimSpace(p.pw)
			a.cacheMu.Lock()
			if old, ok := a.cacheClients[key]; ok {
				_ = old.Disconnect()
				delete(a.cacheClients, key)
			}
			a.cacheMu.Unlock()
			cli2, err2 := a.getClientCached(addr, p.pw)
			if err2 != nil {
				return fmt.Errorf("%s 再接続失敗: %w", addr, err2)
			}
			if _, err3 := cli2.Scenes.SetCurrentProgramScene((&scenes.SetCurrentProgramSceneParams{}).WithSceneName(scene)); err3 != nil {
				return fmt.Errorf("%s 切替失敗: %w", addr, err3)
			}
		}
	}
	return nil
}

func (a *App) enabledPairs() []struct{ addr, pw string } {
	var pairs []struct{ addr, pw string }
	for _, c := range a.cfg.Connections {
		if c.Enabled {
			pairs = append(pairs, struct{ addr, pw string }{addr: c.Addr, pw: strings.TrimSpace(c.Password)})
		}
	}
	return pairs
}

func (a *App) applySceneToEnabledConnections(scene string) error {
	pairs := a.enabledPairs()
	if len(pairs) == 0 {
		return errors.New("有効な接続がありません")
	}
	return a.setSceneCachedPairs(pairs, scene)
}

func (a *App) sceneExistsOnEnabledConnections(scene string) (bool, error) {
	pairs := a.enabledPairs()
	if len(pairs) == 0 {
		return false, errors.New("有効な接続がありません")
	}
	for _, p := range pairs {
		addr := obsws.NormalizeObsAddr(strings.TrimSpace(p.addr))
		if addr == "" {
			return false, errors.New("接続先アドレスが不正です")
		}
		cli, err := a.getClientCached(addr, p.pw)
		if err != nil {
			return false, err
		}
		lst, err := cli.Scenes.GetSceneList(nil)
		if err != nil {
			return false, err
		}
		found := false
		for _, s := range lst.Scenes {
			if s.SceneName == scene {
				found = true
				break
			}
		}
		if !found {
			return false, nil
		}
	}
	return true, nil
}

func (a *App) dispatchScene(scene string, source btsync.Source) error {
	scene = strings.TrimSpace(scene)
	if scene == "" {
		return errors.New("シーン名が空です")
	}

	st := a.bt.Status()
	if st.Running && st.Role == btsync.RoleParent && a.cfg.Bluetooth.Enabled {
		_ = a.emitLog("info", fmt.Sprintf("同期シーン切替を送信: %s (%s)", scene, source))
		return a.bt.DispatchScene(scene, source)
	}

	_ = a.emitLog("info", fmt.Sprintf("ローカルシーン切替: %s (%s)", scene, source))
	return a.applySceneToEnabledConnections(scene)
}

// --- MIDI Support ---

func (a *App) MidiListDevices() ([]string, error) { return midi.ListInputs() }

func (a *App) MidiGetConfig() (config.MidiConfig, error) { return a.cfg.MIDI, nil }

func (a *App) MidiIsRunning() bool { return a.midiDrv != nil }

func (a *App) MidiCurrentDevice() string {
	if a == nil || a.cfg == nil {
		return ""
	}
	return a.cfg.MIDI.Device
}

func (a *App) MidiSaveConfig(mc config.MidiConfig) error {
	a.cfg.MIDI = mc
	if err := config.Save(a.cfg); err != nil {
		return err
	}
	return a.emitLog("info", "MIDI設定を保存しました")
}

func (a *App) MidiStart() error {
	mc := a.cfg.MIDI
	if strings.TrimSpace(mc.Device) == "" {
		return errors.New("MIDIデバイスを選択してください")
	}
	_ = a.MidiStop()

	chs := parseChannels(mc.Channel)
	debounce := mustParseDurationDefault(mc.Debounce, 30*time.Millisecond)
	ratelimit := mustParseDurationDefault(mc.RateLimit, 50*time.Millisecond)
	noteMap := parseNoteMaps(mc.Mappings)

	drv, events, err := midi.OpenInput(mc.Device)
	if err != nil {
		return err
	}
	a.midiDrv = drv
	ctx, cancel := context.WithCancel(context.Background())
	a.midiCancel = cancel
	_ = a.emitLog("info", fmt.Sprintf("MIDI開始: device=%s ch=%s", mc.Device, mc.Channel))

	lastAt := map[string]time.Time{}
	var lastEvent time.Time

	go func() {
		defer func() { _ = a.emitLog("info", "MIDI停止") }()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-events:
				if !ok {
					return
				}
				if !lastEvent.IsZero() && time.Since(lastEvent) < debounce {
					continue
				}
				lastEvent = time.Now()
				if ev.Type != midi.NoteOn {
					continue
				}
				if len(chs) > 0 && !containsChannel(chs, int(ev.Channel)) {
					continue
				}
				key := fmt.Sprintf("%d:%d", ev.Channel, ev.Data1)
				scene, ok := noteMap[key]
				if !ok {
					continue
				}
				if t, ok2 := lastAt[key]; ok2 {
					if since := time.Since(t); since < ratelimit {
						continue
					}
				}
				lastAt[key] = time.Now()
				if err := a.dispatchScene(scene, btsync.SourceMIDI); err != nil {
					_ = a.emitLog("error", fmt.Sprintf("MIDI切替失敗: %v", err))
				} else {
					_ = a.emitLog("info", fmt.Sprintf("MIDI切替: %s (CH%d Note%d)", scene, ev.Channel, ev.Data1))
				}
			}
		}
	}()
	return nil
}

func (a *App) MidiStop() error {
	if a.midiCancel != nil {
		a.midiCancel()
		a.midiCancel = nil
	}
	if a.midiDrv != nil {
		_ = a.midiDrv.Close()
		a.midiDrv = nil
	}
	return nil
}

// --- Bluetooth Sync API ---

func (a *App) BtGetConfig() (config.BluetoothSyncConfig, error) {
	return a.cfg.Bluetooth, nil
}

func (a *App) BtSaveConfig(bc config.BluetoothSyncConfig) error {
	a.cfg.Bluetooth = bc
	if err := config.Save(a.cfg); err != nil {
		return err
	}
	a.bt.SetConfig(btConfigFromGUI(a.cfg.Bluetooth))
	return a.emitLog("info", "Bluetooth同期設定を保存しました")
}

func (a *App) BtStart() error {
	a.bt.SetConfig(btConfigFromGUI(a.cfg.Bluetooth))
	return a.bt.Start()
}

func (a *App) BtStop() error {
	return a.bt.Stop()
}

func (a *App) BtGetStatus() (BluetoothStatus, error) {
	return a.bt.Status(), nil
}

func (a *App) BtGeneratePairingCode() (string, error) {
	return a.bt.GeneratePairingCode()
}

func (a *App) BtJoinByCode(code string) error {
	return a.bt.JoinByCode(code)
}

// --- Helpers ---

func btConfigFromGUI(c config.BluetoothSyncConfig) btsync.Config {
	role := btsync.Role(strings.ToLower(strings.TrimSpace(c.Role)))
	return btsync.Config{
		Enabled:           c.Enabled,
		Role:              role,
		DeviceName:        strings.TrimSpace(c.DeviceName),
		LeadTimeMs:        c.LeadTimeMs,
		PairingCodeTTLSec: c.PairingCodeTTLSec,
		AcceptLateMs:      c.AcceptLateMs,
		MaxNodes:          c.MaxNodes,
		AutoReconnect:     c.AutoReconnect,
		DropMissedEvents:  c.DropMissedEvents,
		TrustedPeers:      toBtTrustedPeers(c.TrustedPeers),
	}
}

func toBtTrustedPeers(peers []config.TrustedPeer) []btsync.TrustedPeer {
	out := make([]btsync.TrustedPeer, 0, len(peers))
	for _, p := range peers {
		out = append(out, btsync.TrustedPeer{
			PeerID:   p.PeerID,
			Name:     p.Name,
			Secret:   p.Secret,
			LastSeen: p.LastSeen,
			Platform: p.Platform,
		})
	}
	return out
}

func toConfigTrustedPeers(peers []btsync.TrustedPeer) []config.TrustedPeer {
	out := make([]config.TrustedPeer, 0, len(peers))
	for _, p := range peers {
		out = append(out, config.TrustedPeer{
			PeerID:   p.PeerID,
			Name:     p.Name,
			Secret:   p.Secret,
			LastSeen: p.LastSeen,
			Platform: p.Platform,
		})
	}
	return out
}

func mustParseDurationDefault(s string, d time.Duration) time.Duration {
	if v, err := time.ParseDuration(strings.TrimSpace(s)); err == nil {
		return v
	}
	return d
}

func parseChannels(s string) []int {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []int
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var v int
		_, err := fmt.Sscanf(p, "%d", &v)
		if err == nil && v >= 1 && v <= 16 {
			out = append(out, v)
		}
	}
	return out
}

func parseNoteMaps(values []string) map[string]string {
	out := map[string]string{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		parts := strings.SplitN(v, "=", 2)
		if len(parts) != 2 {
			continue
		}
		left := strings.TrimSpace(parts[0])
		right := strings.TrimSpace(parts[1])
		if right == "" {
			continue
		}
		ln := strings.Split(left, ":")
		if len(ln) != 2 {
			continue
		}
		ch := strings.TrimSpace(ln[0])
		note := strings.TrimSpace(ln[1])
		var chv, nv int
		if _, err := fmt.Sscanf(ch, "%d", &chv); err != nil || chv < 1 || chv > 16 {
			continue
		}
		if _, err := fmt.Sscanf(note, "%d", &nv); err != nil || nv < 0 || nv > 127 {
			continue
		}
		key := fmt.Sprintf("%d:%d", chv, nv)
		out[key] = right
	}
	return out
}

func containsChannel(list []int, v int) bool {
	if len(list) == 0 {
		return true
	}
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
