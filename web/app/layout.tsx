// web/app/layout.tsx
import "./globals.css";
import { Nav } from "@/components/Nav";

export default async function Root({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        <Nav />
        {children}
      </body>
    </html>
  );
}
