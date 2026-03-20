import { useLocation, useNavigate } from "react-router-dom";
import { ArrowLeft } from "lucide-react";
import { readDetailReturnTo } from "../routing/detailReturnTo";

/** Back arrow when this screen was opened with `navigate(path, { state: { returnTo } })` (e.g. from an agent system graph). */
export function ContextBackButton() {
  const location = useLocation();
  const navigate = useNavigate();
  const returnTo = readDetailReturnTo(location);
  if (!returnTo) return null;
  return (
    <button type="button" className="btn-ghost" onClick={() => navigate(returnTo)} aria-label="Back">
      <ArrowLeft size={16} />
    </button>
  );
}
