import { ArrowLeft } from "lucide-react";

type Props = {
  title: string;
  message: string;
  goBack: () => void;
};

export function ResourceDetailLoadError({ title, message, goBack }: Props) {
  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <button type="button" className="btn-ghost" onClick={goBack} aria-label="Back">
            <ArrowLeft size={16} />
          </button>
          <div>
            <h1 className="page__title">{title}</h1>
            <p className="page__subtitle text-red">{message}</p>
          </div>
        </div>
      </div>
      <p className="text-muted" style={{ marginTop: 16 }}>
        The resource may have been renamed or deleted. Open the list and select it again, or switch namespace if it lives
        elsewhere.
      </p>
    </div>
  );
}
