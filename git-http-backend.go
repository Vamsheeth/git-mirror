package main

import (
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func init() {
	flag.StringVar(&smartconfig.AuthPassEnvVar, "auth_pass_env_var", smartconfig.AuthPassEnvVar, "set an env var to provide the basic auth pass as")
	flag.StringVar(&smartconfig.AuthUserEnvVar, "auth_user_env_var", smartconfig.AuthUserEnvVar, "set an env var to provide the basic auth user as")
	flag.StringVar(&smartconfig.DefaultEnv, "default_env", smartconfig.DefaultEnv, "set the default env")
	flag.StringVar(&smartconfig.ProjectRoot, "project_root", smartconfig.ProjectRoot, "set project root")
	flag.StringVar(&smartconfig.GitBinPath, "git_bin_path", smartconfig.GitBinPath, "set git bin path")
}

// Request handling function

func requestHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s %s", r.RemoteAddr, r.Method, r.URL.Path, r.Proto)
		for match, service := range services {
			re, err := regexp.Compile(match)
			if err != nil {
				log.Print(err)
			}

			if m := re.FindStringSubmatch(r.URL.Path); m != nil {
				if service.Method != r.Method {
					renderMethodNotAllowed(w, r)
					return
				}

				rpc := service.RPC
				file := strings.Replace(r.URL.Path, m[1]+"/", "", 1)
				dir, err := getGitDir(m[1])

				if err != nil {
					log.Print(err)
					renderNotFound(w)
					return
				}

				hr := HandlerReq{w, r, rpc, dir, file}
				service.Handler(hr)
				return
			}
		}
		renderNotFound(w)
		return
	}
}

// Actual command handling functions

func serviceRPC(hr HandlerReq) {
	w, r, rpc, dir := hr.w, hr.r, hr.RPC, hr.Dir
	access := hasAccess(r, dir, rpc, true)

	if access == false {
		renderNoAccess(w)
		return
	}

	w.Header().Set("Content-Type", fmt.Sprintf("application/x-git-%s-result", rpc))
	w.Header().Set("Connection", "Keep-Alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	env := []string{}

	if smartconfig.DefaultEnv != "" {
		env = append(env, smartconfig.DefaultEnv)
	}

	user, password, authok := r.BasicAuth()
	if authok {
		if smartconfig.AuthUserEnvVar != "" {
			env = append(env, fmt.Sprintf("%s=%s", smartconfig.AuthUserEnvVar, user))
		}
		if smartconfig.AuthPassEnvVar != "" {
			env = append(env, fmt.Sprintf("%s=%s", smartconfig.AuthPassEnvVar, password))
		}
	}

	args := []string{rpc, "--stateless-rpc", dir}
	cmd := exec.Command(smartconfig.GitBinPath, args...)
	version := r.Header.Get("Git-Protocol")
	if len(version) != 0 {
		cmd.Env = append(os.Environ(), fmt.Sprintf("GIT_PROTOCOL=%s", version))
	}
	cmd.Dir = dir
	cmd.Env = env
	in, err := cmd.StdinPipe()
	if err != nil {
		log.Print(err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Print(err)
	}

	err = cmd.Start()
	if err != nil {
		log.Print(err)
	}

	var reader io.ReadCloser
	switch r.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err = gzip.NewReader(r.Body)
		defer reader.Close()
	default:
		reader = r.Body
	}
	io.Copy(in, reader)
	in.Close()

	flusher, ok := w.(http.Flusher)
	if !ok {
		panic("expected http.ResponseWriter to be an http.Flusher")
	}

	p := make([]byte, 1024)
	for {
		nRead, err := stdout.Read(p)
		if err == io.EOF {
			break
		}
		nWrite, err := w.Write(p[:nRead])
		if err != nil {
			if strings.Contains(err.Error(), "write: broken pipe") {
				fmt.Println(time.Now().Format(time.RFC850), "Client Connection Closed Ignoring and Continuing....", err)
				break
			}
			fmt.Println(err)
			os.Exit(1)
		}
		if nRead != nWrite {
			fmt.Printf("failed to write data: %d read, %d written\n", nRead, nWrite)
			os.Exit(1)
		}
		flusher.Flush()
	}

	cmd.Wait()
}

func getInfoRefs(hr HandlerReq) {
	w, r, dir := hr.w, hr.r, hr.Dir
	serviceName := getServiceType(r)
	access := hasAccess(r, dir, serviceName, false)
	version := r.Header.Get("Git-Protocol")
	if access {
		args := []string{serviceName, "--stateless-rpc", "--advertise-refs", "."}
		refs := gitCommand(dir, version, args...)

		hdrNocache(w)
		w.Header().Set("Content-Type", fmt.Sprintf("application/x-git-%s-advertisement", serviceName))
		w.WriteHeader(http.StatusOK)
		if len(version) == 0 {
			w.Write(packetWrite("# service=git-" + serviceName + "\n"))
			w.Write(packetFlush())
		}
		w.Write(refs)
	} else {
		updateServerInfo(dir)
		hdrNocache(w)
		sendFile("text/plain; charset=utf-8", hr)
	}
}

func getInfoPacks(hr HandlerReq) {
	hdrCacheForever(hr.w)
	sendFile("text/plain; charset=utf-8", hr)
}

func getLooseObject(hr HandlerReq) {
	hdrCacheForever(hr.w)
	sendFile("application/x-git-loose-object", hr)
}

func getPackFile(hr HandlerReq) {
	hdrCacheForever(hr.w)
	sendFile("application/x-git-packed-objects", hr)
}

func getIdxFile(hr HandlerReq) {
	hdrCacheForever(hr.w)
	sendFile("application/x-git-packed-objects-toc", hr)
}

func getTextFile(hr HandlerReq) {
	hdrNocache(hr.w)
	sendFile("text/plain", hr)
}

// Logic helping functions

func sendFile(contentType string, hr HandlerReq) {
	w, r := hr.w, hr.r
	reqFile := path.Join(hr.Dir, hr.File)

	f, err := os.Stat(reqFile)
	if os.IsNotExist(err) {
		renderNotFound(w)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", f.Size()))
	w.Header().Set("Last-Modified", f.ModTime().Format(http.TimeFormat))
	http.ServeFile(w, r, reqFile)
}

func getGitDir(filePath string) (string, error) {
	root := smartconfig.ProjectRoot

	if root == "" {
		cwd, err := os.Getwd()

		if err != nil {
			log.Print(err)
			return "", err
		}

		root = cwd
	}

	f := path.Join(root, filePath)
	if _, err := os.Stat(f); os.IsNotExist(err) {
		return "", err
	}

	return f, nil
}

func getServiceType(r *http.Request) string {
	serviceType := r.FormValue("service")

	if s := strings.HasPrefix(serviceType, "git-"); !s {
		return ""
	}

	return strings.Replace(serviceType, "git-", "", 1)
}

func hasAccess(r *http.Request, dir string, rpc string, checkContentType bool) bool {
	if checkContentType {
		if r.Header.Get("Content-Type") != fmt.Sprintf("application/x-git-%s-request", rpc) {
			return false
		}
	}

	if !(rpc == "upload-pack" || rpc == "receive-pack") {
		return false
	}
	if rpc == "receive-pack" {
		return smartconfig.ReceivePack
	}
	if rpc == "upload-pack" {
		return smartconfig.UploadPack
	}

	return getConfigSetting(rpc, dir)
}

func getConfigSetting(serviceName string, dir string) bool {
	serviceName = strings.Replace(serviceName, "-", "", -1)
	setting := getGitConfig("http."+serviceName, dir)

	if serviceName == "uploadpack" {
		return setting != "false"
	}

	return setting == "true"
}

func getGitConfig(configName string, dir string) string {
	args := []string{"smartconfig", configName}
	out := string(gitCommand(dir, "", args...))
	return out[0 : len(out)-1]
}

func updateServerInfo(dir string) []byte {
	args := []string{"update-server-info"}
	return gitCommand(dir, "", args...)
}

func gitCommand(dir string, version string, args ...string) []byte {
	command := exec.Command(smartconfig.GitBinPath, args...)
	if len(version) > 0 {
		command.Env = append(os.Environ(), fmt.Sprintf("GIT_PROTOCOL=%s", version))
	}
	command.Dir = dir
	out, err := command.Output()

	if err != nil {
		log.Print(err)
	}

	return out
}

// HTTP error response handling functions

func renderMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	if r.Proto == "HTTP/1.1" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("Method Not Allowed"))
	} else {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad Request"))
	}
}

func renderNotFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte("Not Found"))
}

func renderNoAccess(w http.ResponseWriter) {
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte("Forbidden"))
}

// Packet-line handling function

func packetFlush() []byte {
	return []byte("0000")
}

func packetWrite(str string) []byte {
	s := strconv.FormatInt(int64(len(str)+4), 16)

	if len(s)%4 != 0 {
		s = strings.Repeat("0", 4-len(s)%4) + s
	}

	return []byte(s + str)
}

// Header writing functions

func hdrNocache(w http.ResponseWriter) {
	w.Header().Set("Expires", "Fri, 01 Jan 1980 00:00:00 GMT")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
}

func hdrCacheForever(w http.ResponseWriter) {
	now := time.Now().Unix()
	expires := now + 31536000
	w.Header().Set("Date", fmt.Sprintf("%d", now))
	w.Header().Set("Expires", fmt.Sprintf("%d", expires))
	w.Header().Set("Cache-Control", "public, max-age=31536000")
}

// Main

//func main() {
//	flag.Parse()
//
//	http.Handle("/api/",http.StripPrefix("/api/" ,requestHandler()))
//	err := http.ListenAndServe(address, nil)
//	if err != nil {
//		log.Fatal("ListenAndServe: ", err)
//	}
//}
