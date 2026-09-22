import { useEffect, useMemo, useState } from "react";

type ServiceItem = {
  id: string;
  name: string;
  description?: string;
  group?: string;
  domain: string;
  port: number;
  href: string;
};

type CatalogResponse = {
  baseDomain: string;
  services: ServiceItem[];
};

export default function App() {
  const [catalog, setCatalog] = useState<CatalogResponse | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    const controller = new AbortController();
    fetch("/api/services", { signal: controller.signal })
      .then((response) => {
        if (!response.ok) throw new Error(`API returned ${response.status}`);
        return response.json() as Promise<CatalogResponse>;
      })
      .then(setCatalog)
      .catch((reason: unknown) => {
        if (reason instanceof DOMException && reason.name === "AbortError") return;
        setError(reason instanceof Error ? reason.message : "Unable to load services");
      });
    return () => controller.abort();
  }, []);

  const groups = useMemo(() => {
    const result = new Map<string, ServiceItem[]>();
    for (const service of catalog?.services ?? []) {
      const group = service.group || "Services";
      result.set(group, [...(result.get(group) ?? []), service]);
    }
    return result;
  }, [catalog]);

  return (
    <main>
      <header className="hero">
        <p className="eyebrow">PRIVATE HOME LAB</p>
        <h1>dwPortal</h1>
        <p className="intro">One quiet place for every service in the lab.</p>
      </header>

      {error && (
        <section className="status error" role="alert">
          <strong>服务目录暂不可用</strong>
          <span>{error}</span>
        </section>
      )}

      {!catalog && !error && <p className="status">正在载入服务…</p>}

      {Array.from(groups).map(([group, services]) => (
        <section className="service-group" key={group}>
          <h2>{group}</h2>
          <div className="grid">
            {services.map((service) => (
              <a className="card" href={service.href} key={service.id}>
                <div>
                  <h3>{service.name}</h3>
                  <p>{service.description || service.domain}</p>
                </div>
                <span aria-hidden="true">↗</span>
              </a>
            ))}
          </div>
        </section>
      ))}

      {catalog && <footer>Routing for {catalog.baseDomain}</footer>}
    </main>
  );
}
