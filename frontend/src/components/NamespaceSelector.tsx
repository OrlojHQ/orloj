import { useState, useRef, useEffect, useCallback } from "react";
import { ChevronDown } from "lucide-react";
import { useNamespaces } from "../api/hooks";

interface Props {
  value: string;
  onChange: (ns: string) => void;
}

export function NamespaceSelector({ value, onChange }: Props) {
  const { data: namespaces = [] } = useNamespaces();
  const [open, setOpen] = useState(false);
  const [filter, setFilter] = useState("");
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const filtered = namespaces.filter((ns) =>
    ns.toLowerCase().includes(filter.toLowerCase()),
  );

  const handleSelect = useCallback(
    (ns: string) => {
      onChange(ns);
      setFilter("");
      setOpen(false);
    },
    [onChange],
  );

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setFilter(e.target.value);
    if (!open) setOpen(true);
  };

  const handleInputKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      const target = filter.trim() || value;
      if (filtered.length === 1) {
        handleSelect(filtered[0]);
      } else if (target) {
        handleSelect(target);
      }
      e.preventDefault();
    } else if (e.key === "Escape") {
      setFilter("");
      setOpen(false);
      inputRef.current?.blur();
    } else if (e.key === "ArrowDown" && !open) {
      setOpen(true);
    }
  };

  useEffect(() => {
    function onClickOutside(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
        setFilter("");
      }
    }
    document.addEventListener("mousedown", onClickOutside);
    return () => document.removeEventListener("mousedown", onClickOutside);
  }, []);

  return (
    <div className="ns-selector" ref={containerRef}>
      <div
        className="ns-selector__control"
        onClick={() => {
          setOpen(!open);
          if (!open) inputRef.current?.focus();
        }}
      >
        <input
          ref={inputRef}
          className="ns-selector__input"
          value={open ? filter : value}
          onChange={handleInputChange}
          onFocus={() => setOpen(true)}
          onKeyDown={handleInputKeyDown}
          placeholder={value}
          aria-label="Namespace"
          spellCheck={false}
        />
        <ChevronDown
          size={14}
          className={`ns-selector__chevron ${open ? "ns-selector__chevron--open" : ""}`}
        />
      </div>
      {open && (
        <ul className="ns-selector__menu" role="listbox">
          {filtered.length === 0 && (
            <li className="ns-selector__option ns-selector__option--empty">
              {filter ? "No matches — press Enter to use" : "No namespaces"}
            </li>
          )}
          {filtered.map((ns) => (
            <li
              key={ns}
              className={`ns-selector__option ${ns === value ? "ns-selector__option--active" : ""}`}
              role="option"
              aria-selected={ns === value}
              onClick={() => handleSelect(ns)}
            >
              {ns}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
