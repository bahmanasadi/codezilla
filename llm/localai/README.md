```
docker run -p 8080:8080 --name local-ai -e MODELS="llama3-8b-instruct" -ti localai/localai:latest-aio-cpu


curl http://localhost:8080/v1/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3-8b-instruct",
    "prompt": "Explain quantum computing in simple terms.",
    "max_tokens": 200
  }'

```
