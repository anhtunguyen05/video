package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"video/services/worker/internal/processing/domain"
	"video/services/worker/internal/processing/ports"
)

var ErrInvalidMessage = errors.New("invalid video uploaded message")

const (
	FailureSourceNotFound   = "SOURCE_NOT_FOUND"
	FailureSourceUnreadable = "SOURCE_UNREADABLE"
	FailureInvalidMedia     = "INVALID_MEDIA_STREAM"
	FailureThumbnail        = "THUMBNAIL_GENERATION_FAILED"
	FailureNoRendition      = "NO_SUPPORTED_RENDITION"
	FailureTranscode        = "RENDITION_TRANSCODE_FAILED"
	FailureHLSPackaging     = "HLS_PACKAGING_FAILED"
)

const DefaultThumbnailMaxEdge = 640

type Clock interface {
	Now() time.Time
}

type Service struct {
	repository ports.Repository
	processor  ports.ProcessorRepository
	reliable   ports.ReliableRepository
	source     ports.SourceStorage
	assets     ports.AssetStorage
	probe      ports.MetadataProbe
	thumbnail  ports.ThumbnailGenerator
	transcoder ports.RenditionTranscoder
	packager   ports.HLSVariantPackager
	tempDir    string
	clock      Clock
}

func NewReliableProcessingService(repository ports.Repository, processor ports.ProcessorRepository, reliable ports.ReliableRepository, source ports.SourceStorage, assets ports.AssetStorage, probe ports.MetadataProbe, thumbnail ports.ThumbnailGenerator, transcoder ports.RenditionTranscoder, packager ports.HLSVariantPackager, tempDir string) *Service {
	service := NewProcessingService(repository, processor, source, assets, probe, thumbnail, transcoder, packager, tempDir)
	service.reliable = reliable
	return service
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

type eventEnvelope struct {
	MessageID     string `json:"message_id"`
	MessageType   string `json:"message_type"`
	SchemaVersion int    `json:"schema_version"`
	OccurredAt    string `json:"occurred_at"`
	Payload       struct {
		VideoID           string `json:"video_id"`
		OwnerID           string `json:"owner_id"`
		SourceObjectKey   string `json:"source_object_key"`
		ProcessingVersion int    `json:"processing_version"`
	} `json:"payload"`
}

func NewService(repository ports.Repository) *Service {
	return &Service{repository: repository, clock: realClock{}}
}

func NewServiceWithClock(repository ports.Repository, clock Clock) *Service {
	return &Service{repository: repository, clock: clock}
}

func NewProcessingService(repository ports.Repository, processor ports.ProcessorRepository, source ports.SourceStorage, assets ports.AssetStorage, probe ports.MetadataProbe, thumbnail ports.ThumbnailGenerator, transcoder ports.RenditionTranscoder, packager ports.HLSVariantPackager, tempDir string) *Service {
	return NewProcessingServiceWithClock(repository, processor, source, assets, probe, thumbnail, transcoder, packager, tempDir, realClock{})
}

func NewProcessingServiceWithClock(repository ports.Repository, processor ports.ProcessorRepository, source ports.SourceStorage, assets ports.AssetStorage, probe ports.MetadataProbe, thumbnail ports.ThumbnailGenerator, transcoder ports.RenditionTranscoder, packager ports.HLSVariantPackager, tempDir string, clock Clock) *Service {
	return &Service{
		repository: repository,
		processor:  processor,
		source:     source,
		assets:     assets,
		probe:      probe,
		thumbnail:  thumbnail,
		transcoder: transcoder,
		packager:   packager,
		tempDir:    tempDir,
		clock:      clock,
	}
}

func (service *Service) HandleVideoUploaded(ctx context.Context, body []byte) error {
	job, err := service.queuedJob(body, false)
	if err != nil {
		return err
	}
	if err := service.repository.CreateQueued(ctx, job); err != nil {
		return fmt.Errorf("create queued processing job: %w", err)
	}
	if service.processor == nil {
		return nil
	}
	claimedJob, claimed, err := service.processor.ClaimQueued(ctx, job.OperationKey, service.clock.Now())
	if err != nil {
		return fmt.Errorf("claim queued processing job: %w", err)
	}
	if !claimed {
		return nil
	}
	return service.process(ctx, claimedJob)
}

// EnqueueVideoUploaded is the M9 consumer path. Processing is driven by the
// due-job scheduler, so ACK never depends on the lifetime of FFmpeg.
func (service *Service) EnqueueVideoUploaded(ctx context.Context, body []byte) error {
	job, err := service.queuedJob(body, true)
	if err != nil {
		return err
	}
	if err := service.repository.CreateQueued(ctx, job); err != nil {
		return fmt.Errorf("create queued processing job: %w", err)
	}
	return nil
}

func (service *Service) Quarantine(ctx context.Context, body []byte, reason string) error {
	if service.reliable == nil {
		return errors.New("reliability repository is unavailable")
	}
	return service.reliable.Quarantine(ctx, body, "INVALID_EVENT", reason, service.clock.Now())
}

func (service *Service) ProcessClaimed(ctx context.Context, job domain.Job) error {
	return service.process(ctx, job)
}

func (service *Service) queuedJob(body []byte, strict bool) (domain.Job, error) {
	var event eventEnvelope
	if err := json.Unmarshal(body, &event); err != nil {
		return domain.Job{}, fmt.Errorf("%w: decode JSON: %v", ErrInvalidMessage, err)
	}
	if event.MessageType != "video.uploaded.v1" || event.SchemaVersion != 1 {
		return domain.Job{}, fmt.Errorf("%w: unexpected message type", ErrInvalidMessage)
	}
	if strict {
		if _, err := uuid.Parse(event.MessageID); err != nil {
			return domain.Job{}, fmt.Errorf("%w: invalid message id", ErrInvalidMessage)
		}
		if _, err := uuid.Parse(event.Payload.VideoID); err != nil {
			return domain.Job{}, fmt.Errorf("%w: invalid video id", ErrInvalidMessage)
		}
		if _, err := uuid.Parse(event.Payload.OwnerID); err != nil {
			return domain.Job{}, fmt.Errorf("%w: invalid owner id", ErrInvalidMessage)
		}
		if _, err := time.Parse(time.RFC3339Nano, event.OccurredAt); err != nil {
			return domain.Job{}, fmt.Errorf("%w: invalid occurrence time", ErrInvalidMessage)
		}
	}
	job, err := domain.NewQueuedJob(domain.VideoUploaded{
		VideoID:           event.Payload.VideoID,
		OwnerID:           event.Payload.OwnerID,
		SourceObjectKey:   event.Payload.SourceObjectKey,
		ProcessingVersion: event.Payload.ProcessingVersion,
	}, service.clock.Now())
	if err != nil {
		return domain.Job{}, fmt.Errorf("%w: %v", ErrInvalidMessage, err)
	}
	job.OriginalMessage = append([]byte(nil), body...)
	return job, nil
}

func (service *Service) process(ctx context.Context, job domain.Job) error {
	if service.source == nil || service.assets == nil || service.probe == nil || service.thumbnail == nil || service.transcoder == nil || service.packager == nil {
		return errors.New("processing dependencies are not configured")
	}
	workDir, err := os.MkdirTemp(service.tempDir, "video-processing-")
	if err != nil {
		return fmt.Errorf("create processing temp directory: %w", err)
	}
	defer os.RemoveAll(workDir)

	sourcePath := filepath.Join(workDir, "source")
	sourceFile, err := os.Create(sourcePath)
	if err != nil {
		return fmt.Errorf("create processing source file: %w", err)
	}
	objectInfo, downloadErr := service.source.Download(ctx, job.SourceObjectKey, sourceFile)
	closeErr := sourceFile.Close()
	if downloadErr != nil {
		if errors.Is(downloadErr, ports.ErrObjectNotFound) {
			return service.markFailed(ctx, job, FailureSourceNotFound, "The uploaded source object could not be found.")
		}
		if errors.Is(downloadErr, ports.ErrObjectUnreadable) {
			return service.markFailed(ctx, job, FailureSourceUnreadable, "The uploaded source object could not be read.")
		}
		return fmt.Errorf("download source object: %w", downloadErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close processing source file: %w", closeErr)
	}

	metadata, err := service.probe.Probe(ctx, sourcePath)
	if err != nil {
		if errors.Is(err, ports.ErrInvalidMedia) {
			return service.markFailed(ctx, job, FailureInvalidMedia, "The uploaded media is invalid or missing a video stream.")
		}
		return fmt.Errorf("probe source object: %w", err)
	}
	if err := service.processor.PersistMetadata(ctx, job, metadata, objectInfo.Size, service.clock.Now()); err != nil {
		return fmt.Errorf("persist video metadata: %w", err)
	}
	if err := service.recordOperation(ctx, job, "METADATA", "default", ""); err != nil {
		return err
	}
	renditions, err := domain.PlanRenditions(metadata.Width, metadata.Height)
	if err != nil {
		return fmt.Errorf("plan renditions: %w", err)
	}
	if len(renditions) == 0 {
		return service.markFailed(ctx, job, FailureNoRendition, "The uploaded video is too small for the supported rendition ladder.")
	}

	thumbnailPath := filepath.Join(workDir, "thumbnail.jpg")
	if err := service.thumbnail.Generate(ctx, sourcePath, thumbnailPath, thumbnailTimestamp(metadata.DurationMS)); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return service.markFailed(ctx, job, FailureThumbnail, "The video thumbnail could not be generated.")
	}
	thumbnailInfo, err := os.Stat(thumbnailPath)
	if err != nil || thumbnailInfo.Size() <= 0 {
		return service.markFailed(ctx, job, FailureThumbnail, "The video thumbnail output was empty.")
	}
	thumbnailFile, err := os.Open(thumbnailPath)
	if err != nil {
		return service.markFailed(ctx, job, FailureThumbnail, "The video thumbnail output could not be opened.")
	}
	_, uploadErr := service.assets.Upload(ctx,
		thumbnailObjectKey(job), "image/jpeg", thumbnailFile, thumbnailInfo.Size())
	closeThumbnailErr := thumbnailFile.Close()
	if uploadErr != nil {
		return fmt.Errorf("upload thumbnail: %w", uploadErr)
	}
	if closeThumbnailErr != nil {
		return fmt.Errorf("close thumbnail file: %w", closeThumbnailErr)
	}
	thumbnailAsset := ports.GeneratedAsset{
		ID:          uuid.NewString(),
		VideoID:     job.VideoID,
		AssetType:   "THUMBNAIL",
		Variant:     "default",
		ObjectKey:   thumbnailObjectKey(job),
		ContentType: "image/jpeg",
		SizeBytes:   thumbnailInfo.Size(),
	}
	if err := service.processor.PrepareTranscoding(ctx, job, thumbnailAsset, renditions, service.clock.Now()); err != nil {
		return fmt.Errorf("prepare transcoding: %w", err)
	}
	if err := service.recordOperation(ctx, job, "THUMBNAIL", "default", thumbnailAsset.ObjectKey); err != nil {
		return err
	}
	for _, rendition := range renditions {
		failed, err := service.processRendition(ctx, job, sourcePath, workDir, rendition)
		if err != nil {
			return err
		}
		if failed {
			return nil
		}
	}
	failed, err := service.packageHLS(ctx, job, workDir, renditions, metadata.DurationMS)
	if err != nil {
		return err
	}
	if failed {
		return nil
	}
	if err := service.processor.MarkSucceeded(ctx, job, service.clock.Now()); err != nil {
		return fmt.Errorf("mark processing job succeeded: %w", err)
	}
	return nil
}

func (service *Service) packageHLS(ctx context.Context, job domain.Job, workDir string, renditions []domain.Rendition, durationMS int64) (bool, error) {
	if err := service.processor.BeginPackaging(ctx, job, service.clock.Now()); err != nil {
		return false, fmt.Errorf("begin HLS packaging: %w", err)
	}
	hlsDir := filepath.Join(workDir, "hls")
	if err := os.MkdirAll(hlsDir, 0o700); err != nil {
		return true, service.markHLSPackagingFailed(ctx, job)
	}
	variants := make([]ports.HLSVariant, 0, len(renditions))
	for _, rendition := range renditions {
		inputPath := filepath.Join(workDir, rendition.Name+".mp4")
		variantDir := filepath.Join(hlsDir, rendition.Name)
		if err := os.MkdirAll(variantDir, 0o700); err != nil {
			return true, service.markHLSPackagingFailed(ctx, job)
		}
		if err := service.packager.PackageVariant(ctx, ports.HLSVariantRequest{InputPath: inputPath, OutputDir: variantDir}); err != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			return true, service.markHLSPackagingFailed(ctx, job)
		}
		playlistPath := filepath.Join(variantDir, "index.m3u8")
		playlist, err := os.ReadFile(playlistPath)
		if err != nil || len(playlist) == 0 || !strings.Contains(string(playlist), "#EXT-X-ENDLIST") {
			return true, service.markHLSPackagingFailed(ctx, job)
		}
		inputInfo, err := os.Stat(inputPath)
		if err != nil || inputInfo.Size() <= 0 {
			return true, service.markHLSPackagingFailed(ctx, job)
		}
		bandwidth := int64(inputInfo.Size()) * 8 * 1000 / max64(durationMS, 1)
		variants = append(variants, ports.HLSVariant{
			Name: rendition.Name, Width: rendition.Width, Height: rendition.Height,
			Bandwidth: max64(bandwidth, 1),
		})
	}
	master := buildMasterPlaylist(variants)
	masterPath := filepath.Join(hlsDir, "master.m3u8")
	if err := os.WriteFile(masterPath, master, 0o600); err != nil {
		return true, service.markHLSPackagingFailed(ctx, job)
	}
	files, err := hlsFiles(hlsDir)
	if err != nil {
		return true, service.markHLSPackagingFailed(ctx, job)
	}
	for _, rendition := range renditions {
		segmentCount := 0
		for _, file := range files {
			if filepath.Dir(file) == filepath.Join(hlsDir, rendition.Name) && strings.HasSuffix(file, ".ts") {
				info, statErr := os.Stat(file)
				if statErr != nil || info.Size() == 0 {
					return true, service.markHLSPackagingFailed(ctx, job)
				}
				segmentCount++
			}
		}
		if segmentCount == 0 {
			return true, service.markHLSPackagingFailed(ctx, job)
		}
	}
	uploaded := make([]string, 0, len(files))
	cleanup := func() {
		if job.RunID != "" {
			return
		}
		for _, key := range uploaded {
			_ = service.assets.Delete(context.WithoutCancel(ctx), key)
		}
	}
	var masterAsset ports.GeneratedAsset
	variantAssets := make([]ports.GeneratedAsset, 0, len(variants))
	for _, file := range files {
		fileBytes := fileSize(file)
		if fileBytes <= 0 {
			cleanup()
			return true, service.markHLSPackagingFailed(ctx, job)
		}
		objectKey := hlsObjectKey(job, hlsDir, file)
		handle, openErr := os.Open(file)
		if openErr != nil {
			cleanup()
			return true, service.markHLSPackagingFailed(ctx, job)
		}
		info, uploadErr := service.assets.Upload(ctx, objectKey, hlsContentType(file), handle, fileBytes)
		closeErr := handle.Close()
		if uploadErr != nil || closeErr != nil {
			cleanup()
			if uploadErr != nil && job.RunID != "" {
				return false, fmt.Errorf("upload HLS object: %w", uploadErr)
			}
			if closeErr != nil && job.RunID != "" {
				return false, fmt.Errorf("close HLS object: %w", closeErr)
			}
			return true, service.markHLSPackagingFailed(ctx, job)
		}
		uploaded = append(uploaded, objectKey)
		if strings.HasSuffix(file, ".m3u8") {
			assetType, variant := "HLS_VARIANT", filepath.Base(filepath.Dir(file))
			if filepath.Clean(file) == filepath.Clean(masterPath) {
				assetType, variant = "HLS_MASTER", "default"
			}
			asset := ports.GeneratedAsset{ID: uuid.NewString(), VideoID: job.VideoID, AssetType: assetType, Variant: variant, ObjectKey: objectKey, ContentType: hlsContentType(file), SizeBytes: info.Size}
			if assetType == "HLS_MASTER" {
				masterAsset = asset
			} else {
				variantAssets = append(variantAssets, asset)
			}
		}
	}
	if masterAsset.ObjectKey == "" || len(variantAssets) != len(variants) {
		cleanup()
		return true, service.markHLSPackagingFailed(ctx, job)
	}
	if err := service.processor.CompleteHLS(ctx, job, masterAsset, variantAssets, service.clock.Now()); err != nil {
		if job.RunID == "" {
			cleanup()
		}
		return false, fmt.Errorf("persist HLS assets: %w", err)
	}
	for _, asset := range variantAssets {
		if err := service.recordOperation(ctx, job, "HLS", asset.Variant, asset.ObjectKey); err != nil {
			return false, err
		}
	}
	if err := service.recordOperation(ctx, job, "HLS", "master", masterAsset.ObjectKey); err != nil {
		return false, err
	}
	return false, nil
}

func (service *Service) markHLSPackagingFailed(ctx context.Context, job domain.Job) error {
	if err := service.processor.MarkFailed(ctx, job, FailureHLSPackaging, "The video HLS package could not be generated.", service.clock.Now()); err != nil {
		return fmt.Errorf("mark HLS packaging failed: %w", err)
	}
	return nil
}

func buildMasterPlaylist(variants []ports.HLSVariant) []byte {
	var builder strings.Builder
	builder.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n")
	for _, variant := range variants {
		fmt.Fprintf(&builder, "#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d\n%s/index.m3u8\n", variant.Bandwidth, variant.Width, variant.Height, variant.Name)
	}
	return []byte(builder.String())
}

func hlsFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func hlsObjectKey(job domain.Job, root, path string) string {
	relative, _ := filepath.Rel(root, path)
	if job.RunID != "" {
		return runPrefix(job) + "hls/" + filepath.ToSlash(relative)
	}
	return fmt.Sprintf("users/%s/videos/%s/hls/%s", job.OwnerID, job.VideoID, filepath.ToSlash(relative))
}

func hlsContentType(path string) string {
	if strings.HasSuffix(path, ".m3u8") {
		return "application/vnd.apple.mpegurl"
	}
	return "video/mp2t"
}

func fileSize(path string) int64 {
	info, _ := os.Stat(path)
	if info == nil {
		return 0
	}
	return info.Size()
}
func max64(value, minimum int64) int64 {
	if value < minimum {
		return minimum
	}
	return value
}

func (service *Service) processRendition(ctx context.Context, job domain.Job, sourcePath, workDir string, rendition domain.Rendition) (bool, error) {
	if err := service.processor.BeginRendition(ctx, job, rendition, service.clock.Now()); err != nil {
		return false, fmt.Errorf("begin rendition %s: %w", rendition.Name, err)
	}
	outputPath := filepath.Join(workDir, rendition.Name+".mp4")
	if err := service.transcoder.Transcode(ctx, ports.RenditionTranscodeRequest{
		SourcePath: sourcePath,
		OutputPath: outputPath,
		Width:      rendition.Width,
		Height:     rendition.Height,
	}); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return true, service.markRenditionFailed(ctx, job, rendition, FailureTranscode, "The video rendition could not be generated.")
	}
	outputInfo, err := os.Stat(outputPath)
	if err != nil || outputInfo.Size() <= 0 {
		return true, service.markRenditionFailed(ctx, job, rendition, FailureTranscode, "The video rendition output was empty.")
	}
	outputFile, err := os.Open(outputPath)
	if err != nil {
		return true, service.markRenditionFailed(ctx, job, rendition, FailureTranscode, "The video rendition output could not be opened.")
	}
	objectKey := renditionObjectKey(job, rendition)
	_, uploadErr := service.assets.Upload(ctx, objectKey, "video/mp4", outputFile, outputInfo.Size())
	closeErr := outputFile.Close()
	if uploadErr != nil {
		return false, fmt.Errorf("upload rendition %s: %w", rendition.Name, uploadErr)
	}
	if closeErr != nil {
		return false, fmt.Errorf("close rendition %s: %w", rendition.Name, closeErr)
	}
	asset := ports.GeneratedAsset{
		ID:          uuid.NewString(),
		VideoID:     job.VideoID,
		AssetType:   "RENDITION",
		Variant:     rendition.Name,
		ObjectKey:   objectKey,
		ContentType: "video/mp4",
		SizeBytes:   outputInfo.Size(),
	}
	if err := service.processor.CompleteRendition(ctx, job, asset, rendition, service.clock.Now()); err != nil {
		return false, fmt.Errorf("complete rendition %s: %w", rendition.Name, err)
	}
	if err := service.recordOperation(ctx, job, "TRANSCODE", rendition.Name, asset.ObjectKey); err != nil {
		return false, err
	}
	return false, nil
}

func thumbnailTimestamp(durationMS int64) time.Duration {
	if durationMS <= 0 {
		return 0
	}
	duration := time.Duration(durationMS) * time.Millisecond
	if duration <= time.Second {
		return 0
	}
	timestamp := duration / 10
	latestSafe := duration - 100*time.Millisecond
	if timestamp > latestSafe {
		return latestSafe
	}
	return timestamp
}

func thumbnailObjectKey(job domain.Job) string {
	if job.RunID != "" {
		return runPrefix(job) + "thumbnails/default.jpg"
	}
	return fmt.Sprintf("users/%s/videos/%s/thumbnails/default.jpg", job.OwnerID, job.VideoID)
}

func renditionObjectKey(job domain.Job, rendition domain.Rendition) string {
	if job.RunID != "" {
		return runPrefix(job) + "renditions/" + rendition.Name + "/video.mp4"
	}
	return fmt.Sprintf("users/%s/videos/%s/renditions/%s/video.mp4", job.OwnerID, job.VideoID, rendition.Name)
}

func runPrefix(job domain.Job) string {
	return fmt.Sprintf("users/%s/videos/%s/processing/v%d/runs/%s/", job.OwnerID, job.VideoID, job.ProcessingVersion, job.RunID)
}

func (service *Service) recordOperation(ctx context.Context, job domain.Job, stage, variant, objectKey string) error {
	if service.reliable == nil || job.RunID == "" {
		return nil
	}
	return service.reliable.RecordOperation(ctx, job, stage, variant, objectKey, service.clock.Now())
}

func (service *Service) markFailed(ctx context.Context, job domain.Job, code, message string) error {
	if err := service.processor.MarkFailed(ctx, job, code, message, service.clock.Now()); err != nil {
		return fmt.Errorf("mark processing job failed: %w", err)
	}
	return nil
}

func (service *Service) markRenditionFailed(ctx context.Context, job domain.Job, rendition domain.Rendition, code, message string) error {
	if err := service.processor.MarkRenditionFailed(ctx, job, rendition, code, message, service.clock.Now()); err != nil {
		return fmt.Errorf("mark rendition failed: %w", err)
	}
	return nil
}
