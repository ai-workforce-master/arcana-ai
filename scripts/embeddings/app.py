import os
import numpy as np
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from sentence_transformers import SentenceTransformer

app = FastAPI()

MODEL_NAME = "paraphrase-multilingual-MiniLM-L12-v2"
print(f"🔄 [EMBEDDINGS] ЗАГРУЗКА МОДЕЛИ: {MODEL_NAME}...")
print(f"🔍 [EMBEDDINGS] NumPy version: {np.__version__}")

try:
    model = SentenceTransformer(MODEL_NAME)
    dim = model.get_sentence_embedding_dimension()
    print(f"✅ [EMBEDDINGS] МОДЕЛЬ УСПЕШНО ЗАГРУЖЕНА. Размер вектора: {dim}")
except Exception as e:
    print(f"❌ [EMBEDDINGS] КРИТИЧЕСКАЯ ОШИБКА ЗАГРУЗКИ МОДЕЛИ: {e}")
    raise e

class EmbedRequest(BaseModel):
    text: str

@app.post("/embed")
async def embed(request: EmbedRequest):
    try:
        # Строго локальный эмбеддинг. Фоллбэк убран.
        embedding = model.encode(request.text, convert_to_tensor=False).tolist()
        print(f"✅ [EMBEDDINGS] Локальный эмбеддинг успешен. Размер вектора: {len(embedding)}")
        return {"embedding": embedding, "source": "local"}
    except Exception as e:
        print(f"❌ [EMBEDDINGS] КРИТИЧЕСКАЯ ОШИБКА локального эмбеддинга: {e}")
        raise HTTPException(status_code=500, detail=f"Local embedding failed: {str(e)}")
