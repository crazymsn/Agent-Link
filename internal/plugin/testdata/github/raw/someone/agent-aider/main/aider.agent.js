// Aider (aider.chat) keeps its settings in ~/.aider.conf.yml and reaches
// any OpenAI-compatible endpoint through litellm's openai/ prefix, so
// agentlink's model goes in as openai/<provider>/<model>, beside agentlink's
// address and the key agentlink keeps for Aider.
export const agent = {
  id: "aider",
  name: "Aider",
  bin: "aider",
  config: "~/.aider.conf.yml",
  model: "model",
  prefix: "openai/",
  ua: ["aider", "litellm"],
  notice: "Aider reads its settings when it starts: restart it to use the new model.",
}

export function connect({ gateway }) {
  return {
    "openai-api-base": gateway.v1,
    "openai-api-key": gateway.key,
    // agentlink's model ids aren't in litellm's list, so aider would warn on each
    "show-model-warnings": false,
  }
}
