# API Key Setup Guide

How to obtain an API key for each supported provider and configure it for RaceLLM.

---

## OpenAI

1. Go to [https://platform.openai.com](https://platform.openai.com) and sign in (or create an account).
2. Click your profile icon (top-right) → **Your profile** → **User API keys**.
   Or go directly: [https://platform.openai.com/api-keys](https://platform.openai.com/api-keys)
3. Click **Create new secret key**, give it a name, and copy it immediately — it won't be shown again.
4. Add a payment method: [https://platform.openai.com/settings/organization/billing](https://platform.openai.com/settings/organization/billing)
   OpenAI has no free API tier; you need credits to make calls.

Set the key:
```powershell
# Windows (current session only)
$env:OPENAI_API_KEY = "sk-..."

# Windows (permanent, user-level)
[System.Environment]::SetEnvironmentVariable("OPENAI_API_KEY", "sk-...", "User")
```

---

## Anthropic

1. Go to [https://console.anthropic.com](https://console.anthropic.com) and sign in (or create an account).
2. In the left sidebar, click **API Keys**.
3. Click **Create Key**, give it a name, and copy it immediately.
4. Add credits: **Settings** → **Billing** → **Add credits**.
   Anthropic has no free API tier.

Set the key:
```powershell
# Windows (current session only)
$env:ANTHROPIC_API_KEY = "sk-ant-..."

# Windows (permanent, user-level)
[System.Environment]::SetEnvironmentVariable("ANTHROPIC_API_KEY", "sk-ant-...", "User")
```

---

## Gemini (Google AI Studio)

1. Go to [https://aistudio.google.com](https://aistudio.google.com) and sign in with a Google account.
2. Click **Get API key** (top-left) → **Create API key**.
3. Select or create a Google Cloud project, then click **Create API key in existing project**.
4. Copy the key.

Gemini has a **free tier** with rate limits (e.g., 15 req/min on Flash). No billing required to get started.

Set the key:
```powershell
# Windows (current session only)
$env:GEMINI_API_KEY = "AIza..."

# Windows (permanent, user-level)
[System.Environment]::SetEnvironmentVariable("GEMINI_API_KEY", "AIza...", "User")
```

Then in `racellm.yaml`, set `gemini.enabled: true`.

---

## Ollama (Local — Free, No Key Required)

Ollama runs models on your own machine. No account, no API key, no cost.

1. Download and install from [https://ollama.com/download](https://ollama.com/download).
2. Pull a model:
   ```powershell
   ollama pull llama3.3
   ```
3. Ollama starts a local server automatically at `http://localhost:11434`.

In `racellm.yaml`, set `ollama.enabled: true`. No `api_key` field is needed.

> **Hardware note:** `llama3.3` (70B) requires a capable GPU or will be slow on CPU. For lighter use, try `llama3.2` (3B) instead.

---

## Verifying Your Setup

After setting environment variables (restart your terminal if you used the permanent method), run:

```powershell
go build -o racellm.exe .
.\racellm.exe models
```

This lists every enabled provider/model the app can see. If a key is missing or empty, the API call will return a 401 error at race time.
