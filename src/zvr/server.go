package zvr

import (
	"net/http"
	"fmt"
	"zvr/utils"
	"flag"
	"os"
	"time"
	"encoding/json"
	log "github.com/Sirupsen/logrus"
)

type commandHandlerWrap struct {
	path string
	handler http.HandlerFunc
	async bool
}

type options struct {
	ip string
	port uint
	readTimeout uint
	writeTimeout uint
}

type ReplyHeader struct {
	Success bool `json:"success"`
	Error string `json:"error"`
}

type CommandContext struct {
	responseWriter http.ResponseWriter
	request *http.Request
}

func (ctx *CommandContext) GetCommand(cmd interface{}) {
	if err := utils.JsonDecodeHttpRequest(ctx.request, cmd); err != nil {
		panic(err)
	}
}

type CommandHandler func(ctx *CommandContext) interface{}

type HttpInterceptor func(http.HandlerFunc) http.HandlerFunc

var (
	commandHandlers map[string]*commandHandlerWrap = make(map[string]*commandHandlerWrap)
	commandOptions = &options{}
	interceptors map[string][]HttpInterceptor = make(map[string][]HttpInterceptor, 0)
)

const (
	CALLBACK_URL = "callbackurl"
	TASK_UUID = "taskuuid"
)

func RegisterHttpInterceptor(path string, ic HttpInterceptor)  {
	ics, ok := interceptors[path]
	if !ok {
		ics = make([]HttpInterceptor, 0)
	}

	ics = append(ics, ic)
	interceptors[path] = ics
}

func RegisterSyncCommandHandler(path string, chandler CommandHandler)  {
	registerCommandHandler(path, chandler, false)
}

func RegisterAsyncCommandHandler(path string, chandler CommandHandler) {
	registerCommandHandler(path, chandler, true)
}

func registerCommandHandler(path string, chandler CommandHandler, async bool) {
	utils.Assert(path != "", "path cannot be nil")
	utils.Assert(chandler != nil, "chandler cannot be nil")

	if _, ok := commandHandlers[path]; ok {
		panic(fmt.Errorf("duplicate handler for the path[%v]", path))
	}

	w := &commandHandlerWrap{
		path: path,
		async: async,
	}

	inner := func(w http.ResponseWriter, req *http.Request) {
		ctx := &CommandContext{
			responseWriter: w,
			request: req,
		}

		if !async {
			rsp := chandler(ctx)
			body := ""

			if rsp != nil {
				b, err := json.Marshal(&rsp)
				if err != nil {
					panic(err)
				}
				body = string(b)
			}

			w.WriteHeader(http.StatusOK)
			utils.LogError(fmt.Fprint(w, body))
		} else {
			callbackURL := req.Header.Get(CALLBACK_URL)
			taskUuid := req.Header.Get(TASK_UUID)
			rsp := chandler(ctx)
			utils.Retry(func() error {
				return utils.HttpPostForObject(callbackURL, map[string]string{
					TASK_UUID: taskUuid,
				}, rsp, nil)
			}, 15, 1)
		}
	}

	for _, ics := range interceptors {
		for _, ic := range ics {
			inner = ic(inner)
		}
	}

	w.handler = func(w http.ResponseWriter, req *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				reply := ReplyHeader{
					Success: false,
					Error: fmt.Sprintf("%v", err),
				}

				log.Warnf("command of the path[%s] fails, %v", path, err)

				body, err := json.Marshal(reply)
				if err != nil {
					utils.LogError(err)
					w.WriteHeader(http.StatusInternalServerError)
					utils.LogError(fmt.Fprintf(w, fmt.Sprintf("%s", err)))
					return
				}

				w.WriteHeader(http.StatusOK)
				utils.LogError(fmt.Fprint(w, string(body)))
			}
		}()

		inner(w, req)
	}

	log.Debugf("a command path[%s] is registered", path)
	commandHandlers[path] = w
}

func abortOnWrongOption(msg string) {
	fmt.Println(msg)
	flag.Usage()
	os.Exit(1)
}

func parseCommandOptions()  {
	flag.StringVar(&commandOptions.ip, "ip", "", "The IP address the server listens on")
	flag.UintVar(&commandOptions.port, "port", 7272, "The port the server listens on")
	flag.UintVar(&commandOptions.readTimeout, "readtimeout", 10, "The socket read timeout")
	flag.UintVar(&commandOptions.writeTimeout, "readtimeout", 10, "The socket write timeout")

	flag.Parse()

	if commandOptions.ip == "" {
		abortOnWrongOption("error: the options 'ip' is required")
	}
}

func Start()  {
	parseCommandOptions()
	startServer()
}

type dispatcher func(w http.ResponseWriter, req *http.Request)

func (d dispatcher) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	d(w, req)
}

func dispatch(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Path
	wrap, ok := commandHandlers[path]
	if !ok {
		log.Warnf("no plugin registered the path[%s], drop it", path)
		w.WriteHeader(http.StatusNotFound)
		utils.LogError(fmt.Fprintf(w, "no plugin registered the path[%s]", path))
		return
	}

	if !wrap.async {
		wrap.handler(w, req)
		return
	}

	callbackURL := req.Header.Get(CALLBACK_URL)
	if callbackURL == "" {
		err := fmt.Sprintf("no field '%s' found in the HTTP header but the plugin registers the path[%s]" +
				" as an async command", CALLBACK_URL, path)
		log.Warn(err)
		w.WriteHeader(http.StatusBadRequest)
		utils.LogError(fmt.Fprint(w, err))
		return
	}

	taskUuid := req.Header.Get(TASK_UUID)
	if taskUuid == "" {
		err := fmt.Sprintf("no field '%s' found in the HTTP header but the plugin registers the path[%s]" +
				" as an async command", TASK_UUID, path)
		log.Warn(err)
		w.WriteHeader(http.StatusBadRequest)
		utils.LogError(fmt.Fprint(w, err))
		return
	}

	// for async command, reply first and then handle
	w.WriteHeader(http.StatusOK)
	utils.LogError(fmt.Fprint(w, ""))
	wrap.handler(w, req)
}

func startServer() {
	server := &http.Server{
		Addr: fmt.Sprintf("%v:%v", commandOptions.ip, commandOptions.port),
		ReadTimeout: time.Duration(commandOptions.readTimeout) * time.Second,
		WriteTimeout: time.Duration(commandOptions.writeTimeout) * time.Second,
		Handler: dispatcher(dispatch),
	}

	server.ListenAndServe()
}

func init()  {
	log.SetLevel(log.DebugLevel)
}