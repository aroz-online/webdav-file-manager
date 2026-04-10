package main

import (
	"embed"
	"fmt"
	"net/http"
	"os"
	"strconv"

	plugin "aroz.org/zoraxy/webdav-file-manager/mod/zoraxy_plugin"
)

const (
	PLUGIN_ID = "org.aroz.zoraxy.webdav-file-manager"
	UI_PATH   = "/"
	WEB_ROOT  = "/www"
)

type WebDAVClientConfig struct {
	Enabled       bool   `json:"enabled"`
	Port          int    `json:"port"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	MaxUploadSize int64  `json:"max_upload_size"` // in bytes
}

//go:embed www/*
var content embed.FS

func main() {
	// Serve the plugin intro spect
	runtimeCfg, err := plugin.ServeAndRecvSpec(&plugin.IntroSpect{
		ID:            PLUGIN_ID,
		Name:          "WebDAV File Manager",
		Author:        "aroz.org",
		AuthorContact: "noreply@aroz.org",
		Description:   "A WebDAV file manager for accessing Zoraxy Static Web Server",
		URL:           "https://zoraxy.aroz.org",
		Type:          plugin.PluginType_Utilities,
		VersionMajor:  1,
		VersionMinor:  0,
		VersionPatch:  0,
		UIPath:        UI_PATH,
	})
	if err != nil {
		panic(err)
	}

	// Initialize WebDAV client from config
	if err := initWebDAVClient(); err != nil {
		fmt.Println("Warning: Failed to initialize WebDAV client:", err)
	}

	// Create a new PluginEmbedUIRouter
	embedWebRouter := plugin.NewPluginEmbedUIRouter(PLUGIN_ID, &content, WEB_ROOT, UI_PATH)
	embedWebRouter.RegisterTerminateHandler(func() {
		stopAllWebDAVServerConnections()
		fmt.Println("WebDAV File Manager plugin exited")
		os.Exit(0)
	}, nil)

	// Get and set WebDAV server connection configurations
	embedWebRouter.HandleFunc("/api/setConfigs", handleSetConfigs, nil)
	embedWebRouter.HandleFunc("/api/getConfigs", handleGetConfigs, nil)

	// Web file manager apis
	embedWebRouter.HandleFunc("/api/file/list", handleList, nil)
	embedWebRouter.HandleFunc("/api/file/open", handleOpenFile, nil)
	embedWebRouter.HandleFunc("/api/file/delete", handleDeleteFile, nil)
	embedWebRouter.HandleFunc("/api/file/upload", handleUploadFile, nil)
	embedWebRouter.HandleFunc("/api/file/download", handleDownloadFile, nil)
	embedWebRouter.HandleFunc("/api/file/rename", handleRenameFile, nil)
	embedWebRouter.HandleFunc("/api/file/move", handleMoveFile, nil)
	embedWebRouter.HandleFunc("/api/file/cut", handleCutFile, nil)
	embedWebRouter.HandleFunc("/api/file/newFolder", handleNewFolder, nil)
	embedWebRouter.HandleFunc("/api/file/save", handleSaveFile, nil)

	// Serve the WebDAV file manager page
	http.Handle(UI_PATH, embedWebRouter.Handler())
	fmt.Println("WebDAV File Manager plugin started at http://127.0.0.1:" + strconv.Itoa(runtimeCfg.Port))
	err = http.ListenAndServe("127.0.0.1:"+strconv.Itoa(runtimeCfg.Port), nil)
	if err != nil {
		panic(err)
	}
}
