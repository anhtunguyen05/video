import { HealthStatus } from "@/components/health-status";

export default function HomePage() {
  return (
    <section className="hero">
      <p className="eyebrow">Runtime foundation</p>
      <h1>Video workspace</h1>
      <p className="lede">
        A small dashboard shell for the asynchronous video platform.
      </p>
      <HealthStatus />
    </section>
  );
}

