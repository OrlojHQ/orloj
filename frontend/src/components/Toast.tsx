import { useEffect, useState, useCallback } from "react";
import { X, CheckCircle, AlertTriangle, XCircle, Info } from "lucide-react";
import clsx from "clsx";

export type ToastType = "success" | "error" | "warning" | "info";

interface ToastItem {
  id: number;
  type: ToastType;
  message: string;
}

let toastId = 0;
const listeners: Set<(t: ToastItem) => void> = new Set();

export function toast(type: ToastType, message: string) {
  const item: ToastItem = { id: ++toastId, type, message };
  listeners.forEach((fn) => fn(item));
}

const ICONS: Record<ToastType, React.ReactNode> = {
  success: <CheckCircle size={16} />,
  error: <XCircle size={16} />,
  warning: <AlertTriangle size={16} />,
  info: <Info size={16} />,
};

export function ToastContainer() {
  const [toasts, setToasts] = useState<ToastItem[]>([]);

  const addToast = useCallback((item: ToastItem) => {
    setToasts((prev) => [...prev, item]);
    setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== item.id));
    }, 4000);
  }, []);

  useEffect(() => {
    listeners.add(addToast);
    return () => { listeners.delete(addToast); };
  }, [addToast]);

  const dismiss = (id: number) => setToasts((prev) => prev.filter((t) => t.id !== id));

  if (toasts.length === 0) return null;

  return (
    <div className="toast-container">
      {toasts.map((t) => (
        <div key={t.id} className={clsx("toast", `toast--${t.type}`)}>
          <span className="toast__icon">{ICONS[t.type]}</span>
          <span className="toast__message">{t.message}</span>
          <button className="toast__close" onClick={() => dismiss(t.id)} aria-label="Dismiss">
            <X size={14} />
          </button>
        </div>
      ))}
    </div>
  );
}
