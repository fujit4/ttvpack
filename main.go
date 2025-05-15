package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
)

// ```plugins.yml
// start:
//   - repo: username/repo1
//     tag: v1.0.0
//     url: https://github.com/username/repo1/archive/refs/tags/v1.0.0.zip
//   - repo: username/repo2
//     branch: main
//     url: https://github.com/username/repo2/archive/refs/heads/main.zip
//
// opt:
//   - repo: username/repo3
//     branch: main
//     url: https://github.com/username/repo3/archive/refs/heads/main.zip
// ```

type Plugin struct {
	Repo string `yaml:"repo"`
	Tag  string `yaml:"tag"`
	Branch  string `yaml:"branch"`
	Url  string `yaml:"url"`
}

type Plugins struct {
	Start []Plugin `yaml:"start"`
	Opt   []Plugin `yaml:"opt"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "エラー: %v\n", err)
		os.Exit(1)
	}
}

func run() error {

	// plugins.ymlの取得
	pluginsFilePath := getPluginsFilePath()
	fmt.Println(pluginsFilePath)

	// packフォルダパスの取得
	err, packPath := getPackDir()
	if err != nil {
		return err
	}
	fmt.Println(packPath)

	if len(os.Args) < 1 {
		return errors.New("コマンドを指定してください。")
	}
	cmd := os.Args[1]
	switch cmd {
	case "add":
		add()
	case "rm":
		remove()
	case "sync":
		return sync(pluginsFilePath, packPath)
	default:
		return errors.New("存在しないコマンドです。")
	}

	return nil
}

func add() error {
	fmt.Println("add")
	return nil
}

func remove() error {
	fmt.Println("remove")
	return nil
}

func sync(pluginsFilePath, packPath string) error {
	fmt.Println("start sync")

	startPath := filepath.Join(packPath, "start")
	optPath := filepath.Join(packPath, "opt")

	plugins, err := readPlugins(pluginsFilePath)
	if err != nil {
		return err
	}
	startPluginsMap := makePluginsMap(plugins.Start)
	// optPluginsMap := makePluginsMap(plugins.Opt)

	// 前処理
	os.MkdirAll(startPath, 0755)
	os.MkdirAll(optPath, 0755)

	// ゴミ掃除
	fmt.Println("remove not used plugins")
	// startフォルダのディレクトリの1階層のみをwalkし、リストを作る
	existedStartPlugins, err := listDirEntries(startPath)
	if err != nil {
		return err
	}

	// ディレクトリリストをループし、pluginsの中に存在しない場合は、ディレクトリを削除する
	for _, entry := range existedStartPlugins {
		if _, ok := startPluginsMap[filepath.Base(entry)]; ok {
			// exist
		} else {
			// not exist
			if err := os.RemoveAll(entry); err != nil {
				return err
			}
			fmt.Println("removed: ", filepath.Base(entry))
		}
	}

	// optフォルダのディレクトリの1階層のみをwalkし、リストを作る
	// ディレクトリリストをループし、pluginsの中に存在しない場合は、ディレクトリを削除する

	// インストール
	// startのpluginsをループ
	existedStartPlugins, err = listDirEntries(startPath)
	if err != nil {
		return err
	}

	for _, p := range plugins.Start {
		dirName := makeDirName(p)
		if slices.Contains(existedStartPlugins, dirName) {
			continue
		}


		zipPath := filepath.Join(startPath, dirName+".zip")
		if err := downloadZip(p.Url, zipPath); err != nil {
			return err
		}

		fmt.Println("zip ", zipPath)
		expandedPath := filepath.Join(startPath, dirName)
		// unzip(zipPath, expandedPath)
		// unzip(zipPath, ".")
		if err := unzipWithoutTopLevel(zipPath, expandedPath); err != nil {
			return err
		}
		if err := os.Remove(zipPath); err != nil {
			return err
		}
		fmt.Println("installed ", dirName)
	}

	// startフォルダのリストに存在しなければ、ダウンロードする
	// ダウンロードできたら、非同期でzip解凍を行う
	// optのpluginsをループし
	// optフォルダのリストに存在しなければ、ダウンロードする
	// ダウンロードできたら、非同期でzip解凍を行う

	return nil
}

func listDirEntries(dirPath string) ([]string, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, entry := range entries {
		fullPath := filepath.Join(dirPath, entry.Name())
		paths = append(paths, fullPath)
	}
	return paths, nil
}

func readPlugins(path string) (*Plugins, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var plugins Plugins
	if err := yaml.Unmarshal(data, &plugins); err != nil {
		return nil, err
	}

	return &plugins, nil
}

func makePluginsMap(plugins []Plugin) map[string]string {
	pluginsMap := make(map[string]string)
	for _, p := range plugins {
		key := makeDirName(p)
		pluginsMap[key] = p.Tag
	}
	return pluginsMap
}

func getPackDir() (error, string) {
	cmd := exec.Command("nvim", "--headless", "-c", "lua io.stdout:write(vim.o.packpath)", "-c", "qa")
	output, err := cmd.Output()
	if err != nil {
		return err, ""
	}
	dir := filepath.Join(string(output), "pack", "ttpack")
	return nil, dir
}

func getPluginsFilePath() string {

	fileName := "plugins.yml"

	// XDG_CONFIG_HOMEの取得
	xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, "nvim", fileName)
	}

	switch runtime.GOOS {
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData != "" {
			return filepath.Join(localAppData, "nvim", fileName)
		} else {
			return fileName
		}
	default:
		return filepath.Join("~", ".config", "nvim", fileName)
	}

}

func makeDirName(plugin Plugin) string {
	dir := path.Base(plugin.Repo)

	// if plugin.Tag != "" {
	// 	dir = dir + "-" + plugin.Tag
	// } else if plugin.Branch != "" {
	// 	dir = dir + "-" + plugin.Branch
	// }
	return dir
}

// func makeUrl(plugin Plugin) (string, error) {
// 	targetUrl := ""
// 	var err error
// 	baseUrl := "https://github.com/"
// 	if plugin.Tag == "master" || plugin.Tag == "main" {
// 		targetUrl, err = url.JoinPath(baseUrl, plugin.Repo, "archive/refs/heads/", plugin.Tag+".zip")
// 		if err != nil {
// 			return targetUrl, err
// 		}
// 	} else {
// 		targetUrl, err = url.JoinPath(baseUrl, plugin.Repo, "archive/refs/tags/", plugin.Tag+".zip")
// 		return targetUrl, err
// 	}
// 	return targetUrl, err
// }

func downloadZip(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("zip ファイルのオープンに失敗しました: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		// Zip スリップ攻撃を防ぐためのパス検証
		fpath := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("不正なファイルパス: %s", fpath)
		}

		if f.FileInfo().IsDir() {
			// ディレクトリの作成
			if err := os.MkdirAll(fpath, os.ModePerm); err != nil {
				return fmt.Errorf("ディレクトリの作成に失敗しました: %w", err)
			}
			continue
		}

		// 親ディレクトリの作成
		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return fmt.Errorf("親ディレクトリの作成に失敗しました: %w", err)
		}

		// 出力ファイルの作成
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return fmt.Errorf("出力ファイルの作成に失敗しました: %w", err)
		}
		defer outFile.Close()

		// zip ファイル内のファイルを開く
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("zip 内のファイルのオープンに失敗しました: %w", err)
		}
		defer rc.Close()

		// ファイルの内容をコピー
		if _, err := io.Copy(outFile, rc); err != nil {
			return fmt.Errorf("ファイルのコピーに失敗しました: %w", err)
		}
	}

	return nil
}


func unzipWithoutTopLevel(src, dest string) error {
    r, err := zip.OpenReader(src)
    if err != nil {
        return err
    }
    defer r.Close()

    // トップレベルディレクトリ名を特定
    topLevelDir := ""
    for _, f := range r.File {
        parts := strings.Split(f.Name, "/")
        if len(parts) > 1 {
            if topLevelDir == "" {
                topLevelDir = parts[0]
            } else if topLevelDir != parts[0] {
                topLevelDir = ""
                break
            }
        } else {
            topLevelDir = ""
            break
        }
    }

    for _, f := range r.File {
        // トップレベルディレクトリを除外
        relPath := f.Name
        if topLevelDir != "" {
            if strings.HasPrefix(f.Name, topLevelDir+"/") {
                relPath = strings.TrimPrefix(f.Name, topLevelDir+"/")
            } else {
                // 一致しない場合はそのまま
                relPath = f.Name
            }
        }

        fpath := filepath.Join(dest, relPath)

        if f.FileInfo().IsDir() {
            os.MkdirAll(fpath, os.ModePerm)
            continue
        }

        if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
            return err
        }

        outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
        if err != nil {
            return err
        }

        rc, err := f.Open()
        if err != nil {
            outFile.Close()
            return err
        }

        _, err = io.Copy(outFile, rc)

        outFile.Close()
        rc.Close()

        if err != nil {
            return err
        }
    }
    return nil
}
