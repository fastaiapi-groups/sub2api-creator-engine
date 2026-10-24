// 仅供服务器运维使用；使用容器已有数据库连接，不生成或导出用户 Token。
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	_ "github.com/jackc/pgx/v5/stdlib"
	"os"
	"strings"
	"sub-adds/backend/internal/clientupdates"
	"time"
)

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func main() {
	action := flag.String("action", "", "create/upload/manifest/validate/publish/list")
	id := flag.Int64("id", 0, "发布记录编号")
	version := flag.String("version", "", "版本号")
	notes := flag.String("notes", "", "发布说明")
	targets := flag.String("targets", "macos/arm64,windows/x64", "目标平台")
	target := flag.String("target", "", "上传平台")
	file := flag.String("file", "", "安装包或签名清单")
	name := flag.String("name", "", "安装包文件名")
	out := flag.String("out", "", "清单输出文件")
	flag.Parse()
	if os.Getenv("CLIENT_UPDATES_ENABLED") != "true" {
		must(fmt.Errorf("更新服务未启用"))
	}
	db, e := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	must(e)
	defer db.Close()
	public, e := clientupdates.ParsePublic(os.Getenv("CLIENT_UPDATES_PUBLIC_KEY"))
	must(e)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	service, e := clientupdates.New(ctx, db, nil, os.Getenv("CLIENT_UPDATES_DIR"), public)
	must(e)
	// actor=0 在审计中明确代表服务器运维操作，不冒用站点管理员账号。
	var result any
	switch *action {
	case "create":
		result, e = service.Create(ctx, 0, *version, *notes, strings.Split(*targets, ","))
		must(e)
	case "upload":
		parts := strings.Split(*target, "/")
		if len(parts) != 2 {
			must(fmt.Errorf("平台格式无效"))
		}
		f, e := os.Open(*file)
		must(e)
		defer f.Close()
		result, e = service.Upload(ctx, *id, 0, parts[0], parts[1], *name, f)
		must(e)
	case "manifest":
		r, e := service.Get(ctx, *id)
		must(e)
		m := clientupdates.DraftManifest(r)
		must(m.Validate())
		result = m
	case "validate":
		b, e := os.ReadFile(*file)
		must(e)
		var env clientupdates.Envelope
		must(json.Unmarshal(b, &env))
		result, e = service.Validate(ctx, *id, 0, env)
		must(e)
	case "publish":
		must(service.Publish(ctx, *id, 0))
		result = map[string]bool{"published": true}
	case "list":
		result, e = service.List(ctx)
		must(e)
	default:
		must(fmt.Errorf("未知操作"))
	}
	if *out != "" {
		b, e := json.MarshalIndent(result, "", "  ")
		must(e)
		must(os.WriteFile(*out, append(b, '\n'), 0600))
		fmt.Println("已保存发布清单")
	} else {
		must(json.NewEncoder(os.Stdout).Encode(result))
	}
}
