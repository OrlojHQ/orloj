import Editor from "@monaco-editor/react";
import { useAppStore } from "../store";

interface YamlEditorProps {
  value: string;
  onChange?: (value: string) => void;
  readOnly?: boolean;
  height?: string;
}

export function YamlEditor({ value, onChange, readOnly = true, height = "400px" }: YamlEditorProps) {
  const theme = useAppStore((s) => s.theme);

  return (
    <div className="yaml-editor">
      <Editor
        height={height}
        language="yaml"
        value={value}
        onChange={(v) => onChange?.(v ?? "")}
        theme={theme === "dark" ? "vs-dark" : "light"}
        options={{
          readOnly,
          minimap: { enabled: false },
          fontSize: 13,
          fontFamily: "'JetBrains Mono', 'SF Mono', monospace",
          lineNumbers: "on",
          scrollBeyondLastLine: false,
          wordWrap: "on",
          padding: { top: 12 },
          renderLineHighlight: "none",
          overviewRulerBorder: false,
        }}
      />
    </div>
  );
}
