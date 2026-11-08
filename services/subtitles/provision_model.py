"""Download a pinned multilingual model snapshot for offline inference."""
import os
from huggingface_hub import snapshot_download

snapshot_download(repo_id="Systran/faster-whisper-small",
                  revision="536b0662742c02347bc0e980a01041f333bce120",
                  local_dir=os.getenv("SUBTITLE_MODEL_DIR", "/models/whisper-small"),
                  allow_patterns=["config.json", "model.bin", "tokenizer.json", "vocabulary.txt", "preprocessor_config.json"])
