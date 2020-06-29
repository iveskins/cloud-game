package nanoarch

import (
	"crypto/md5"
	"fmt"
	"image"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/giongto35/cloud-game/pkg/config"
)

// Emulator data mock.
type EmulatorMock struct {
	naEmulator

	// Libretro compiled lib core name
	core string
	// draw canvas instance
	canvas *image.RGBA
	// shared core paths (can't be changed)
	paths EmulatorPaths

	// channels
	imageInCh  <-chan GameFrame
	audioInCh  <-chan []int16
	inputOutCh chan<- InputEvent
}

// Defines various emulator file paths.
type EmulatorPaths struct {
	assets string
	cores  string
	games  string
	save   string
}

// Returns a properly stubbed emulator instance.
// Due to extensive use globals, it should exist in one instance per a test run.
// Don't forget to init one image channel consumer, it will lock-out otherwise.
// Make sure you call shutdownEmulator().
func GetEmulatorMock(room string, system string) *EmulatorMock {
	assetsPath := getAssetsPath()
	metadata := config.EmulatorConfig[system]

	images := make(chan GameFrame, 30)
	audio := make(chan []int16, 30)
	inputs := make(chan InputEvent, 100)

	// an emu
	emu := &EmulatorMock{
		naEmulator: naEmulator{
			imageChannel: images,
			audioChannel: audio,
			inputChannel: inputs,

			meta:           metadata,
			controllersMap: map[string][]controllerState{},
			roomID:         room,
			done:           make(chan struct{}, 1),
			lock:           &sync.Mutex{},
		},

		canvas: image.NewRGBA(image.Rect(0, 0, metadata.Width, metadata.Height)),
		core:   path.Base(metadata.Path),

		paths: EmulatorPaths{
			assets: cleanPath(assetsPath),
			cores:  cleanPath(assetsPath + "emulator/libretro/cores/"),
			games:  cleanPath(assetsPath + "games/"),
		},

		imageInCh:  images,
		audioInCh:  audio,
		inputOutCh: inputs,
	}

	// stub globals
	NAEmulator = &emu.naEmulator
	outputImg = emu.canvas

	emu.paths.save = cleanPath(emu.GetHashPath())

	return emu
}

// Returns initialized emulator mock with default params.
// Spawns audio/image channels consumers.
// Don't forget to close emulator mock with shutdownEmulator().
func GetDefaultEmulatorMock(room string, system string, rom string) *EmulatorMock {
	mock := GetEmulatorMock(room, system)
	mock.loadRom(rom)
	go mock.handleVideo(func(_ GameFrame) {})
	go mock.handleAudio(func(_ []int16) {})

	return mock
}

// Load a rom into the emulator.
// The rom will be loaded from emulators' games path.
func (emu *EmulatorMock) loadRom(game string) {
	fmt.Printf("%v %v\n", emu.paths.cores, emu.core)
	coreLoad(emu.paths.cores+emu.core, false, false, "")
	coreLoadGame(emu.paths.games + game)
}

// Close the emulator and cleanup its resources.
func (emu *EmulatorMock) shutdownEmulator() {
	_ = os.Remove(emu.GetHashPath())

	close(emu.imageChannel)
	close(emu.audioChannel)
	close(emu.inputOutCh)

	nanoarchShutdown()
}

// Emulate one frame with exclusive lock.
func (emu *EmulatorMock) emulateOneFrame() {
	emu.GetLock()
	nanoarchRun()
	emu.ReleaseLock()
}

// Who needs generics anyway?
// Custom handler for the video channel.
func (emu *EmulatorMock) handleVideo(handler func(image GameFrame)) {
	for frame := range emu.imageInCh {
		handler(frame)
	}
}

// Custom handler for the audio channel.
func (emu *EmulatorMock) handleAudio(handler func(sample []int16)) {
	for frame := range emu.audioInCh {
		handler(frame)
	}
}

// Custom handler for the input channel.
func (emu *EmulatorMock) handleInput(handler func(event InputEvent)) {
	for event := range emu.inputChannel {
		handler(event)
	}
}

// Returns the full path to the emulator latest save.
func (emu *EmulatorMock) getSavePath() string {
	return cleanPath(emu.GetHashPath())
}

// Returns the current emulator state and
// the latest saved state for its session.
// Locks the emulator.
func (emu *EmulatorMock) dumpState() (string, string) {
	emu.GetLock()
	bytes, _ := ioutil.ReadFile(emu.paths.save)
	persistedStateHash := getHash(bytes)
	emu.ReleaseLock()

	stateHash := emu.getStateHash()
	fmt.Printf("mem: %v, dat: %v\n", stateHash, persistedStateHash)
	return stateHash, persistedStateHash
}

// Returns the current emulator state hash.
// Locks the emulator.
func (emu *EmulatorMock) getStateHash() string {
	emu.GetLock()
	state, _ := getState()
	emu.ReleaseLock()

	return getHash(state)
}

// Returns absolute path to the assets directory.
func getAssetsPath() string {
	appName := "cloud-game"
	var (
		// get app path at runtime
		_, b, _, _ = runtime.Caller(0)
		basePath   = filepath.Dir(strings.SplitAfter(b, appName)[0]) + "/" + appName + "/assets/"
	)

	return basePath
}

// Returns MD5 hash.
func getHash(bytes []byte) string {
	return fmt.Sprintf("%x", md5.Sum(bytes))
}

// Returns a proper file path for current OS.
func cleanPath(path string) string {
	return filepath.FromSlash(path)
}
