import { useNavigate } from "react-router-dom";
import { FileQuestion } from "lucide-react";

export function NotFound() {
  const navigate = useNavigate();

  return (
    <div className="page">
      <div className="not-found">
        <FileQuestion size={48} className="not-found__icon" />
        <h1 className="not-found__title">Page Not Found</h1>
        <p className="not-found__message">
          The page you're looking for doesn't exist or has been moved.
        </p>
        <button className="btn-primary" onClick={() => navigate("/")}>
          Go to Dashboard
        </button>
      </div>
    </div>
  );
}
