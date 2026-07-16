FROM python:3.10-slim

WORKDIR /app

COPY requirements.txt .

# Устанавливаем зависимости с жесткими версиями для гарантии совместимости
RUN pip install --no-cache-dir -r requirements.txt

COPY app.py .

# Проверка окружения прямо во время сборки образа
RUN python -c "import numpy; print('BUILD CHECK NumPy:', numpy.__version__); import torch; print('BUILD CHECK Torch:', torch.__version__); from sentence_transformers import SentenceTransformer; m = SentenceTransformer('paraphrase-multilingual-MiniLM-L12-v2'); print('BUILD CHECK Model dim:', m.get_sentence_embedding_dimension())"

EXPOSE 8000

CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8000"]
