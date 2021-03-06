package gobrake

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func newNotifierFilter(notifier *Notifier) func(*Notice) *Notice {
	opt := notifier.opt
	return func(notice *Notice) *Notice {
		if opt.Environment != "" {
			notice.Context["environment"] = opt.Environment
		}
		if opt.Revision != "" {
			notice.Context["revision"] = opt.Revision
		}
		return notice
	}
}

func NewBlocklistKeysFilter(keys ...interface{}) func(*Notice) *Notice {
	return func(notice *Notice) *Notice {
		for _, key := range keys {
			notice.Env = filterByKey(notice.Env, key)
			notice.Context = filterByKey(notice.Context, key)
			notice.Session = filterByKey(notice.Session, key)
		}

		return notice
	}
}

func filterByKey(values map[string]interface{}, key interface{}) map[string]interface{} {
	const filtered = "[Filtered]"

	switch key := key.(type) {
	case string:
		for k := range values {
			if k == key {
				values[k] = filtered
			}
		}
	case *regexp.Regexp:
		for k := range values {
			if key.MatchString(k) {
				values[k] = filtered
			}
		}
	default:
		panic(fmt.Errorf("unsupported blacklist key type: %T", key))
	}

	return values
}

func gopathFilter(notice *Notice) *Notice {
	s, ok := notice.Context["gopath"].(string)
	if !ok {
		return notice
	}

	dirs := filepath.SplitList(s)
	for i := range notice.Errors {
		backtrace := notice.Errors[i].Backtrace
		for j := range backtrace {
			frame := &backtrace[j]

			for _, dir := range dirs {
				dir = filepath.Join(dir, "src")
				if strings.HasPrefix(frame.File, dir) {
					frame.File = strings.Replace(frame.File, dir, "/GOPATH", 1)
					break
				}
			}
		}
	}

	return notice
}

func gitFilter(notice *Notice) *Notice {
	rootDir, _ := notice.Context["rootDirectory"].(string)
	if rootDir == "" {
		return notice
	}

	gitDir, ok := findGitDir(rootDir)
	if !ok {
		return notice
	}

	info := getGitInfo(gitDir)

	if notice.Context == nil {
		notice.Context = make(map[string]interface{})
	}

	if notice.Context["repository"] == nil && info.Repository != "" {
		notice.Context["repository"] = info.Repository
	}

	if notice.Context["revision"] == nil && info.Revision != "" {
		notice.Context["revision"] = info.Revision
	}

	if info.LastCheckout != nil {
		notice.Context["lastCheckout"] = info.LastCheckout
	}

	return notice
}

func httpUnsolicitedResponseFilter(notice *Notice) *Notice {
	if notice.Errors[0].Type != "string" {
		return notice
	}

	prefix := "Unsolicited response received on idle HTTP channel starting with"
	if !strings.HasPrefix(notice.Errors[0].Message, prefix) {
		return notice
	}

	return nil
}

func codeHunksFilter(notice *Notice) *Notice {
	for i := range notice.Errors {
		error := &notice.Errors[i]
		for j := range error.Backtrace {
			frame := &error.Backtrace[j]
			code, err := getCode(frame.File, frame.Line)
			if err != nil {
				if !os.IsNotExist(err) {
					logger.Printf("getCode file=%q line=%d failed: %s",
						frame.File, frame.Line, err)
				}
				continue
			}
			frame.Code = code
		}
	}
	return notice
}
