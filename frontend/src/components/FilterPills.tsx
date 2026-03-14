import clsx from "clsx";

interface FilterPillsProps {
  options: { value: string; label: string; count?: number }[];
  selected: string;
  onSelect: (value: string) => void;
}

export function FilterPills({ options, selected, onSelect }: FilterPillsProps) {
  return (
    <div className="filter-pills">
      {options.map((opt) => (
        <button
          key={opt.value}
          className={clsx("filter-pill", selected === opt.value && "filter-pill--active")}
          onClick={() => onSelect(opt.value)}
        >
          {opt.label}
          {opt.count !== undefined && <span className="filter-pill__count">{opt.count}</span>}
        </button>
      ))}
    </div>
  );
}
