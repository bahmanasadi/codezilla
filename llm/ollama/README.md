docker run -d -v ollama:`pwd`/ollama-data -p 11434:11434 --name ollama ollama/ollama
docker exec -it ollama ollama run qwen2.5-coder:3b

curl http://localhost:11434/api/generate -d '{
  "model": "qwen2.5-coder:3b",
  "prompt": "What are the health benefits of walking daily?",
  "system": "Give the response in JSON format.",
  "stream": false
}'

curl http://localhost:11434/api/generate -d '{
  "model": "qwen3:4b",
  "prompt": "What are the health benefits of walking daily?",
  "system": "Give the response in JSON format.",
  "stream": false
}'

