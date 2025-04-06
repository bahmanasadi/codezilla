docker run -d -v ollama:`pwd`/ollama-data -p 11434:11434 --name ollama ollama/ollama
docker exec -it ollama ollama run dolphin3:8b

curl http://localhost:11434/api/generate -d '{
  "model": "dolphin3:8b",
  "prompt": "What are the health benefits of walking daily?",
  "system": "Give the response in JSON format.",
  "stream": false
}'

