// 仅供服务器运维使用；使用容器已有数据库连接，不生成或导出用户 Token。
package main

import (
	"context"
	"crypto/ed25519"
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
	product := flag.String("product", clientupdates.Product, "产品标识")
	mandatory := flag.Bool("mandatory", false, "是否强制更新（笑笑漫剧）")
	out := flag.String("out", "", "清单输出文件")
	flag.Parse()
	if *product != clientupdates.Product && *product != clientupdates.ManjuProduct {
		must(fmt.Errorf("产品标识无效"))
	}
	if *product == clientupdates.Product && os.Getenv("CLIENT_UPDATES_ENABLED") != "true" {
		must(fmt.Errorf("更新服务未启用"))
	}
	db, e := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	must(e)
	defer db.Close()
	var public ed25519.PublicKey
	var private ed25519.PrivateKey
	if *product == clientupdates.ManjuProduct {
		private, e = clientupdates.LoadSigningKey(os.Getenv("MANJU_UPDATES_SIGNING_KEY_FILE"))
		must(e)
		public = private.Public().(ed25519.PublicKey)
	} else {
		public, e = clientupdates.ParsePublic(os.Getenv("CLIENT_UPDATES_PUBLIC_KEY"))
		must(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	service, e := clientupdates.New(ctx, db, nil, os.Getenv("CLIENT_UPDATES_DIR"), public)
	must(e)
	service.Product = *product
	service.SigningKey = private
	// actor=0 在审计中明确代表服务器运维操作，不冒用站点管理员账号。
	var result any
	switch *action {
	case "create":
		result, e = service.CreateWithOptions(ctx, 0, *version, *notes, strings.Split(*targets, ","), clientupdates.ReleaseOptions{Mandatory: *mandatory})
		must(e)
	case "prepare":
		result, e = service.Prepare(ctx, *id, 0)
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
