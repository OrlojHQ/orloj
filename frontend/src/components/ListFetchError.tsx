import { AlertTriangle } from "lucide-react";

interface ListFetchErrorProps {
  message: string;
  onRetry?: () => void;
}

export function ListFetchError({ message, onRetry }: ListFetchErrorProps) {
  return (
    <div className="list-fetch-error">
      <AlertTriangle size={20} />
      <p>{message}</p>
      {onRetry && (
        <button type="button" className="btn-secondary" onClick={onRetry}>
          Retry
        </button>
      )}
    </div>
  );
}
