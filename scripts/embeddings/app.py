import os
import httpx
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from sentence_transformers import SentenceTransformer

app = FastAPI()

# Загрузка лёгкой мультиязычной модели (~470MB)
MODEL_NAME = "paraphrase-multilingual-MiniLM-L12-v2"
model = SentenceTransformer(MODEL_NAME)

AITUNNEL_API_KEY = os.getenv("FALLBACK_AITUNNEL_API_KEY", "")
AITUNNEL_BASE_URL = os.getenv("FALLBACK_AITUNNEL_BASE_URL", "https://api.aitunnel.ru/v1/")

class EmbedRequest(BaseModel):
    text: str

@app.post("/embed")
async def embed(request: EmbedRequest):
    try:
        # Попытка локального эмбеддинга
        embedding = model.encode(request.text, convert_to_tensor=False).tolist()
        return {"embedding": embedding, "source": "local"}
    except Exception as e:
        # Фоллбэк на AITunnel при ошибке локальной модели
        print(f"Local embedding failed, falling back to AITunnel: {e}")
        return await fallback_to_aitunnel(request.text)

async def fallback_to_aitunnel(text: str):
    headers = {
        "Authorization": f"Bearer {AITUNNEL_API_KEY}",
        "Content-Type": "application/json"
    }
    payload = {
        "model": "text-embedding-3-small",
        "input": text
    }
    async with httpx.AsyncClient() as client:
        response = await client.post(
            f"{AITUNNEL_BASE_URL}/embeddings",
            headers=headers,
            json=payload
        )
        if response.status_code != 200:
            raise HTTPException(status_code=500, detail="Both local and fallback embedding failed")
        
        data = response.json()
        return {"embedding": data["data"][0]["embedding"], "source": "aitunnel_fallback"}
