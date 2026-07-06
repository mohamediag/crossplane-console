import { useState } from "react";
import { PrismLight as SyntaxHighlighter } from "react-syntax-highlighter";
import yaml from "react-syntax-highlighter/dist/esm/languages/prism/yaml";
import { oneLight } from "react-syntax-highlighter/dist/esm/styles/prism";

SyntaxHighlighter.registerLanguage("yaml", yaml);

export function YamlTab({ yaml: source }: { yaml: string }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    await navigator.clipboard.writeText(source);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };
  return (
    <div className="relative">
      <button
        onClick={copy}
        className="absolute right-3 top-2 z-10 rounded bg-zinc-800 px-2 py-1 text-xs text-white hover:bg-zinc-700"
      >
        {copied ? "Copied ✓" : "Copy"}
      </button>
      <SyntaxHighlighter
        language="yaml"
        style={oneLight}
        customStyle={{ margin: 0, fontSize: "12px", background: "transparent" }}
        showLineNumbers
      >
        {source || "# no manifest available"}
      </SyntaxHighlighter>
    </div>
  );
}
