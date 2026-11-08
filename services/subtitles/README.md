# Subtitle engine (development only)

This contains the batch ASR engine, request intake, job executor and result
publisher. Run migrations through 12 before starting them. The end-to-end
browser and Vietnamese-quality acceptance checks in `docs/21_AI_SUBTITLES.md` remain
required before treating the feature as verified.

From the repository root, after the usual infrastructure and migrations:

```powershell
docker compose --profile subtitle-setup run --rm subtitle-model
docker compose --profile subtitles up -d subtitle-intake subtitle-worker subtitle-results
```

The first command downloads the multilingual model into the `subtitle_models`
volume, pinned to revision `536b0662742c02347bc0e980a01041f333bce120`. It needs
network access and disk space. The worker uses CPU INT8 and reads weights from
that volume; inference does not send audio or subtitles to a model provider.

Use a Python environment with the pinned `requirements.txt`, an installed
`ffprobe`, and a locally provisioned multilingual Whisper `small` model in
faster-whisper/CTranslate2 format. Record the source and immutable revision of
the model files used; the runner requires a local directory including
`tokenizer.json` and does not download model or tokenizer files. Do not use
`small.en` for Vietnamese.

```powershell
python -m pip install -r services/subtitles/requirements.txt
python services/subtitles/transcribe.py path/to/input.mp4 path/to/new-output.vtt --model-dir path/to/model
```

The runner uses CPU INT8 and prints only `READY`, `NO_AUDIO`, or `NO_SPEECH` on
success. It refuses to overwrite the output. Subtitle files contain private
content: use consented input and keep outputs out of shared logs and source
control. This runner does not clean up a partially written output after a disk
error; remove that file before retrying.

Unit tests do not require a model, network or FFmpeg:

```powershell
cd services/subtitles
python -B -m unittest discover -s tests -v
```

From the repository root, `make test-python` runs the same discovery command.
The three integration tests require a disposable PostgreSQL database with all
repository migrations applied and a disposable RabbitMQ broker. Set
`TEST_DATABASE_URL` and `TEST_RABBITMQ_URL` before running the command to include
them. Never point these variables at shared or production services: the tests
consume messages from fixed queues and create and remove database records.

The Python CI workflow provisions isolated services, applies migrations, runs
the suite with skips treated as failures, and builds the subtitle image. It
does not download the model or verify real transcription and browser playback.
