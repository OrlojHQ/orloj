import { useState, useSyncExternalStore } from "react";
import Editor from "@monaco-editor/react";
import { useAppStore } from "../store";
import { Pencil, Save, X } from "lucide-react";

const mqMobile = typeof window !== "undefined" ? window.matchMedia("(max-width: 768px)") : null;
function useIsMobile() {
  return useSyncExternalStore(
    (cb) => { mqMobile?.addEventListener("change", cb); return () => mqMobile?.removeEventListener("change", cb); },
    () => mqMobile?.matches ?? false,
  );
}

interface YamlEditorProps {
  value: string;
  onChange?: (value: string) => void;
  readOnly?: boolean;
  height?: string;
  editable?: boolean;
  onSave?: (body: unknown) => Promise<void>;
}

export function YamlEditor({ value, onChange, readOnly = true, height = "400px", editable, onSave }: YamlEditorProps) {
  const theme = useAppStore((s) => s.theme);
  const isMobile = useIsMobile();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(value);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const isEditable = editable && onSave;
  const isReadOnly = isEditable ? !editing : readOnly;

  const handleEdit = () => {
    setDraft(value);
    setError(null);
    setEditing(true);
  };

  const handleCancel = () => {
    setEditing(false);
    setError(null);
  };

  const handleSave = async () => {
    try {
      const body = JSON.parse(draft);
      setSaving(true);
      setError(null);
      await onSave!(body);
      setEditing(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Invalid JSON");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="yaml-editor">
      {isEditable && (
        <div className="yaml-editor__toolbar">
          {!editing ? (
            <button className="btn-secondary btn-sm" onClick={handleEdit}>
              <Pencil size={13} /> Edit
            </button>
          ) : (
            <>
              <button className="btn-primary btn-sm" onClick={handleSave} disabled={saving}>
                <Save size={13} /> {saving ? "Saving..." : "Save"}
              </button>
              <button className="btn-secondary btn-sm" onClick={handleCancel} disabled={saving}>
                <X size={13} /> Cancel
              </button>
            </>
          )}
          {error && <span className="yaml-editor__error">{error}</span>}
        </div>
      )}
      <Editor
        height={height}
        language="yaml"
        value={editing ? draft : value}
        onChange={(v) => {
          const val = v ?? "";
          if (editing) setDraft(val);
          onChange?.(val);
        }}
        theme={theme === "dark" ? "vs-dark" : "light"}
        options={{
          readOnly: isReadOnly,
          minimap: { enabled: false },
          fontSize: isMobile ? 15 : 13,
          fontFamily: "'JetBrains Mono', 'SF Mono', monospace",
          lineNumbers: isMobile ? "off" : "on",
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
