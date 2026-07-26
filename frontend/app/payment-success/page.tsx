'use client';

import { useEffect, useState, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { motion } from 'framer-motion';

const BACKEND_URL = process.env.NEXT_PUBLIC_BACKEND_URL || 'https://ai-arcana.ru';

const CARD_MAP: Record<string, number> = {
  "Шут": 0, "Маг": 1, "Верховная Жрица": 2, "Императрица": 3, "Император": 4, "Иерофант": 5, "Влюбленные": 6, "Колесница": 7, "Сила": 8, "Отшельник": 9,
  "Колесо Фортуны": 10, "Справедливость": 11, "Повешенный": 12, "Смерть": 13, "Умеренность": 14, "Дьявол": 15, "Башня": 16, "Звезда": 17, "Луна": 18, "Солнце": 19,
  "Суд": 20, "Мир": 21, "Туз Кубков": 22, "Двойка Кубков": 23, "Тройка Кубков": 24, "Четверка Кубков": 25, "Пятерка Кубков": 26, "Шестерка Кубков": 27, "Семерка Кубков": 28, "Восьмерка Кубков": 29,
  "Девятка Кубков": 30, "Десятка Кубков": 31, "Паж Кубков": 32, "Рыцарь Кубков": 33, "Королева Кубков": 34, "Король Кубков": 35, "Туз Пентаклей": 36, "Двойка Пентаклей": 37, "Тройка Пентаклей": 38, "Четверка Пентаклей": 39,
  "Пятерка Пентаклей": 40, "Шестерка Пентаклей": 41, "Семерка Пентаклей": 42, "Восьмерка Пентаклей": 43, "Девятка Пентаклей": 44, "Десятка Пентаклей": 45, "Паж Пентаклей": 46, "Рыцарь Пентаклей": 47, "Королева Пентаклей": 48, "Король Пентаклей": 49,
  "Туз Мечей": 50, "Двойка Мечей": 51, "Тройка Мечей": 52, "Четверка Мечей": 53, "Пятерка Мечей": 54, "Шестерка Мечей": 55, "Семерка Мечей": 56, "Восьмерка Мечей": 57, "Девятка Мечей": 58, "Десятка Мечей": 59, "Паж Мечей": 60, "Рыцарь Мечей": 61, "Королева Мечей": 62, "Король Мечей": 63,
  "Туз Жезлов": 64, "Двойка Жезлов": 65, "Тройка Жезлов": 66, "Четверка Жезлов": 67, "Пятерка Жезлов": 68, "Шестерка Жезлов": 69, "Семерка Жезлов": 70, "Восьмерка Жезлов": 71, "Девятка Жезлов": 72, "Десятка Жезлов": 73, "Паж Жезлов": 74, "Рыцарь Жезлов": 75, "Королева Жезлов": 76, "Король Жезлов": 77
};

interface InterpretationData {
  card: { name: string; is_reversed: boolean };
  interpretation: string;
}

interface ReadingResult {
  intro: string;
  cards: InterpretationData[];
  conclusion: string;
}

function PaymentSuccessContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  
  const urlPaymentId = searchParams.get('payment_id');
  const localPaymentId = typeof window !== 'undefined' ? localStorage.getItem('arcana_payment_id') : null;
  const paymentId = urlPaymentId || localPaymentId;
  
  const [status, setStatus] = useState<'checking' | 'showing' | 'failed' | 'not_found'>('checking');
  const [result, setResult] = useState<ReadingResult | null>(null);
  const [revealedCards, setRevealedCards] = useState<InterpretationData[]>([]);
  const [currentCardIndex, setCurrentCardIndex] = useState(0);
  const [error, setError] = useState('');

  useEffect(() => {
    console.log('PaymentSuccess loaded. paymentId:', paymentId, 'urlPaymentId:', urlPaymentId, 'localPaymentId:', localPaymentId);
    
    if (!paymentId) {
      console.log('No paymentId found. Showing reading directly as fallback.');
      setStatus('showing');
      startReading();
      return;
    }

    // Всегда проверяем статус платежа, если paymentId есть
    const checkStatus = async () => {
      try {
        console.log('Checking payment status for:', paymentId);
        const res = await fetch(`${BACKEND_URL}/api/payment-status/${paymentId}`);
        if (!res.ok) {
          console.error('Payment status check failed:', res.status);
          setStatus('not_found');
          return;
        }
        const data = await res.json();
        console.log('Payment status response:', data);
        
        if (data.status === 'succeeded') {
          console.log('Payment succeeded! Starting reading...');
          setStatus('showing');
          startReading();
        } else if (data.status === 'canceled' || data.status === 'refunded') {
          setStatus('failed');
        } else {
          console.log('Payment still pending, checking again in 3s...');
          setTimeout(checkStatus, 3000);
        }
      } catch (err) {
        console.error('Error checking payment status:', err);
        setTimeout(checkStatus, 3000);
      }
    };
    
    checkStatus();
  }, [paymentId]);

  const startReading = async () => {
    console.log('startReading called');
    const question = localStorage.getItem('arcana_question');
    const spreadStr = localStorage.getItem('arcana_spread');
    
    console.log('localStorage data:', { question, spreadStr });
    
    if (!question || !spreadStr) {
      setError('Данные расклада не найдены. Пожалуйста, начните заново.');
      return;
    }

    const spread = JSON.parse(spreadStr);
    const allCards = Object.keys(CARD_MAP);
    
    const shuffled = [...allCards].sort(() => Math.random() - 0.5);
    const selectedCards = shuffled.slice(0, spread.cardCount).map(name => ({
      name,
      is_reversed: Math.random() < 0.3
    }));

    console.log('Selected cards:', selectedCards);

    try {
      const response = await fetch(`${BACKEND_URL}/api/read-spread`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ question, spread: spread.name, cards: selectedCards }),
      });

      if (!response.ok) {
        const errText = await response.text();
        throw new Error(`Failed to get reading: ${response.status} - ${errText}`);
      }
      const data: ReadingResult = await response.json();
      console.log('Reading result received:', data);
      setResult(data);
      revealCardsSequentially(data.cards);
    } catch (err: any) {
      console.error('Error getting reading:', err);
      setError(`Не удалось получить интерпретацию: ${err.message}`);
    }
  };

  const revealCardsSequentially = async (cards: InterpretationData[]) => {
    for (let i = 0; i < cards.length; i++) {
      await new Promise(resolve => setTimeout(resolve, 2500));
      setRevealedCards(prev => [...prev, cards[i]]);
      setCurrentCardIndex(i + 1);
    }
  };

  const handleNewReading = () => {
    localStorage.removeItem('arcana_question');
    localStorage.removeItem('arcana_spread');
    localStorage.removeItem('arcana_session_id');
    localStorage.removeItem('arcana_payment_id');
    router.push('/');
  };

  const getCardImage = (cardName: string) => {
    const index = CARD_MAP[cardName] || 0;
    return `https://ai-arcana.ru/cards/RWS_${String(index).padStart(5, '0')}.jpg`;
  };

  if (status === 'checking') {
    return (
      <div className="min-h-screen flex items-center justify-center p-4">
        <motion.div className="glass p-8 text-center" initial={{ opacity: 0, scale: 0.9 }} animate={{ opacity: 1, scale: 1 }}>
          <motion.div className="text-6xl mb-4" animate={{ rotate: 360 }} transition={{ duration: 2, repeat: Infinity, ease: "linear" }}></motion.div>
          <h2 className="text-2xl text-yellow-400 mb-2">Проверяем оплату...</h2>
        </motion.div>
      </div>
    );
  }

  if (status === 'failed' || status === 'not_found') {
    return (
      <div className="min-h-screen flex items-center justify-center p-4">
        <motion.div className="glass p-8 text-center" initial={{ opacity: 0 }}>
          <div className="text-6xl mb-4">❌</div>
          <h2 className="text-2xl text-red-400 mb-2">Оплата не найдена или отменена</h2>
          <button onClick={() => router.push('/')} className="btn-gold mt-4">Вернуться на главную</button>
        </motion.div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="min-h-screen flex items-center justify-center p-4">
        <motion.div className="glass p-8 text-center" initial={{ opacity: 0 }}>
          <div className="text-6xl mb-4">⚠️</div>
          <h2 className="text-2xl text-yellow-400 mb-2">Ошибка</h2>
          <p className="text-gray-300 mb-4">{error}</p>
          <button onClick={handleNewReading} className="btn-gold">Начать заново</button>
        </motion.div>
      </div>
    );
  }

  return (
    <div className="min-h-screen py-8 px-4">
      <div className="max-w-5xl mx-auto">
        <motion.header className="text-center mb-8" initial={{ opacity: 0, y: -30 }} animate={{ opacity: 1, y: 0 }}>
          <h1 className="text-4xl md:text-5xl font-bold mb-2 text-yellow-400">✨ Ваш расклад готов ✨</h1>
        </motion.header>
        <motion.div className="glass p-8 md:p-12" initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }}>
          {!result ? (
            <div className="text-center py-12">
              <motion.div className="text-6xl mb-4" animate={{ scale: [1, 1.2, 1] }} transition={{ duration: 1.5, repeat: Infinity }}>🔮</motion.div>
              <p className="text-xl text-gray-300">Запрашиваем мудрость ИИ...</p>
            </div>
          ) : (
            <>
              {result.intro && (
                <motion.div className="mb-8 p-6 glass" initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
                  <p className="text-lg leading-relaxed">{result.intro}</p>
                </motion.div>
              )}
              <div className="space-y-6 mb-8">
                {revealedCards.map((card, index) => (
                  <motion.div key={index} className="glass p-6" initial={{ opacity: 0, x: -50 }} animate={{ opacity: 1, x: 0 }}>
                    <div className="flex flex-col md:flex-row gap-6">
                      <motion.div initial={{ rotateY: 180 }} animate={{ rotateY: 0 }} transition={{ duration: 0.8 }} style={{ perspective: 1000 }}>
                        <img src={getCardImage(card.card.name)} alt={card.card.name} className="w-40 md:w-48 h-auto rounded-lg shadow-2xl border-2 border-yellow-400/30" />
                      </motion.div>
                      <div className="flex-1">
                        <h3 className="text-2xl mb-2 text-yellow-400">Позиция {index + 1}</h3>
                        <p className="text-xl mb-3 font-bold text-gray-100">
                          {card.card.name} {card.card.is_reversed && <span className="text-red-400">🔄 (перевёрнутая)</span>}
                        </p>
                        <p className="text-gray-200 leading-relaxed">{card.interpretation}</p>
                      </div>
                    </div>
                  </motion.div>
                ))}
              </div>
              {currentCardIndex < result.cards.length && (
                <div className="text-center mb-8">
                  <motion.div className="text-5xl" animate={{ scale: [1, 1.3, 1] }} transition={{ duration: 1.5, repeat: Infinity }}>✨</motion.div>
                  <p className="text-gray-300 mt-2">Раскрываем следующую карту... ({currentCardIndex}/{result.cards.length})</p>
                </div>
              )}
              {currentCardIndex === result.cards.length && result.conclusion && (
                <motion.div className="glass p-8 mb-8 border-2 border-yellow-400/40" initial={{ opacity: 0, y: 30 }} animate={{ opacity: 1, y: 0 }}>
                  <h3 className="text-3xl mb-4 text-center text-yellow-400">✨ Итог ✨</h3>
                  <p className="text-lg leading-relaxed text-gray-100">{result.conclusion}</p>
                </motion.div>
              )}
              {currentCardIndex === result.cards.length && (
                <div className="text-center">
                  <p className="text-xl text-yellow-400 mb-6">✨ Если у вас есть ещё вопросы, начните новый расклад!</p>
                  <motion.button onClick={handleNewReading} className="btn-gold text-lg pulse-gold" whileHover={{ scale: 1.05 }} whileTap={{ scale: 0.95 }}>
                    🔮 Новый расклад
                  </motion.button>
                </div>
              )}
            </>
          )}
        </motion.div>
      </div>
    </div>
  );
}

export default function PaymentSuccessPage() {
  return (
    <Suspense fallback={
      <div className="min-h-screen flex items-center justify-center p-4">
        <motion.div className="glass p-8 text-center" initial={{ opacity: 0, scale: 0.9 }} animate={{ opacity: 1, scale: 1 }}>
          <motion.div className="text-6xl mb-4" animate={{ rotate: 360 }} transition={{ duration: 2, repeat: Infinity, ease: "linear" }}>⏳</motion.div>
          <h2 className="text-2xl text-yellow-400 mb-2">Загрузка...</h2>
        </motion.div>
      </div>
    }>
      <PaymentSuccessContent />
    </Suspense>
  );
}
