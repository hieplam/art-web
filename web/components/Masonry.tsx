import type { ReactNode } from "react";

export function Masonry({ children }: { children: ReactNode }) {
  return <div className="masonry">{children}</div>;
}
