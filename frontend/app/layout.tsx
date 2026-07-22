import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: 'Arcana AI — Мудрость Таро с ИИ',
  description: 'Персональные расклады Таро с интерпретацией от искусственного интеллекта. Анонимно, точно, глубоко.',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="ru">
      <body>
        <StarryBackground />
        <main className="relative z-10 min-h-screen">
          {children}
        </main>
      </body>
    </html>
  );
}

function StarryBackground() {
  const stars = Array.from({ length: 120 }, (_, i) => ({
    id: i,
    left: Math.random() * 100,
    top: Math.random() * 100,
    size: Math.random() * 2 + 1,
    delay: Math.random() * 3,
  }));

  return (
    <div className="starry-sky">
      {stars.map((star) => (
        <div
          key={star.id}
          className="star"
          style={{
            left: `${star.left}%`,
            top: `${star.top}%`,
            width: `${star.size}px`,
            height: `${star.size}px`,
            animationDelay: `${star.delay}s`,
          }}
        />
      ))}
    </div>
  );
}
