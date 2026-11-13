package clientupdates

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

const UploadChunkBytes = int64(8 << 20)

var uploadIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type UploadProgress struct {
	Complete   bool     `json:"complete"`
	NextOffset int64    `json:"next_offset"`
	Release    *Release `json:"release,omitempty"`
}

// Small, ordered chunks fit through CDN upload limits. A row lock makes retries
// idempotent, and the committed offset recovers an interrupted file write.
func (s *Service) UploadChunk(ctx context.Context, releaseID, actor int64, uploadID, platform, arch, name string, offset, total int64, reader io.Reader) (UploadProgress, error) {
	if s.product() != ManjuProduct || !uploadIDPattern.MatchString(uploadID) || offset < 0 || total <= 0 || total > 2<<30 || offset >= total || !filenamePattern.MatchString(name) || (platform == "renderer" && total > 64<<20) {
		return UploadProgress{}, errors.New("上传分片参数无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.Get(ctx, releaseID)
	if err != nil {
		return UploadProgress{}, err
	}
	if !s.canEdit(r) {
		return UploadProgress{}, errors.New("当前版本无法上传文件")
	}
	declared := false
	for _, target := range r.Required {
		if target == platform+"/"+arch {
			declared = true
		}
	}
	if !declared {
		return UploadProgress{}, errors.New("该文件平台不在本次发布范围内")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return UploadProgress{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO client_release_uploads(id,product_id,release_id,actor,platform,arch,name,total_bytes) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(id) DO NOTHING`, uploadID, s.product(), releaseID, actor, platform, arch, name, total); err != nil {
		return UploadProgress{}, err
	}
	var product, savedPlatform, savedArch, savedName string
	var savedRelease, savedActor, savedTotal, received int64
	var complete bool
	err = tx.QueryRowContext(ctx, `SELECT product_id,release_id,actor,platform,arch,name,total_bytes,received_bytes,complete FROM client_release_uploads WHERE id=$1 FOR UPDATE`, uploadID).Scan(&product, &savedRelease, &savedActor, &savedPlatform, &savedArch, &savedName, &savedTotal, &received, &complete)
	if err != nil {
		return UploadProgress{}, err
	}
	if product != s.product() || savedRelease != releaseID || savedActor != actor || savedPlatform != platform || savedArch != arch || savedName != name || savedTotal != total {
		return UploadProgress{}, errors.New("上传编号与当前文件不匹配")
	}
	if complete {
		return UploadProgress{Complete: true, NextOffset: total, Release: &r}, nil
	}
	expected := min(UploadChunkBytes, total-offset)
	if offset < received && offset+expected <= received {
		return UploadProgress{NextOffset: received}, nil
	}
	if offset != received {
		return UploadProgress{}, errors.New("上传进度不一致，请重新选择文件上传")
	}
	chunk, err := io.ReadAll(io.LimitReader(reader, expected+1))
	if err != nil {
		return UploadProgress{}, err
	}
	if int64(len(chunk)) != expected {
		return UploadProgress{}, errors.New("上传分片长度不完整")
	}
	filename := filepath.Join(s.Root, "upload-"+uploadID+".part")
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return UploadProgress{}, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return UploadProgress{}, err
	}
	if info.Size() < received {
		f.Close()
		return UploadProgress{}, errors.New("上传暂存文件不完整，请重新上传")
	}
	if err = f.Truncate(received); err != nil {
		f.Close()
		return UploadProgress{}, err
	}
	_, err = f.WriteAt(chunk, received)
	if err == nil {
		err = f.Sync()
	}
	f.Close()
	if err != nil {
		return UploadProgress{}, err
	}
	next := received + int64(len(chunk))
	progress := UploadProgress{NextOffset: next, Complete: next == total}
	if progress.Complete {
		f, err := os.Open(filename)
		if err != nil {
			return UploadProgress{}, err
		}
		release, err := s.upload(ctx, tx, releaseID, actor, platform, arch, name, f)
		f.Close()
		if err != nil {
			return UploadProgress{}, err
		}
		progress.Release = &release
	}
	if _, err = tx.ExecContext(ctx, `UPDATE client_release_uploads SET received_bytes=$1,complete=$2,updated_at=now() WHERE id=$3`, next, progress.Complete, uploadID); err != nil {
		return UploadProgress{}, err
	}
	if err = tx.Commit(); err != nil {
		return UploadProgress{}, err
	}
	if progress.Complete {
		_ = os.Remove(filename)
	}
	return progress, nil
}
