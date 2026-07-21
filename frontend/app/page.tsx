'use client';

import { useState } from 'react';
import { spreads, Spread } from '@/lib/spreads';
import { getCardImageUrl, cardNameToNumber } from '@/lib/cards';

type Step = 
  | 'welcome'
  | 'question'
  | 'spread-select'
  | 'payment'
  | 'payment-confirm'
  | 'shuffling'
  | 'cutting'
  | 'revealing'
  | 'result';

interface CardData {
  name: string;
  is_reversed: boolean;
}

interface InterpretationData {
  card: CardData;
  interpretation: string;
}

interface ReadingResult {
  intro: string;
  cards: InterpretationData[];
  conclusion: string;
}

export default function HomePage() {
  const [step, setStep] = useState<Step>('welcome');
  const [question, setQuestion] = useState('');
  const [selectedSpread, setSelectedSpread] = useState<Spread | null>(null);
  const [paymentToken, setPaymentToken] = useState('');
  const [revealedCards, setRevealedCards] = useState<InterpretationData[]>([]);
  const [currentCardIndex, setCurrentCardIndex] = useState(0);
  const [result, setResult] = useState<ReadingResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const handleStart = () => {
    setStep('question');
  };

  const handleQuestionSubmit = () => {
    if (question.trim().length < 3) {
      setError('Пожалуйста, задайте более конкретный вопрос');
      return;
    }
    setError('');
    setStep('spread-select');
  };

  const handleSpreadSelect = (spread: Spread) => {
    setSelectedSpread(spread);
    const token = `TOKEN_${Date.now()}_${Math.random().toString(36).substring(7)}`;
    setPaymentToken(token);
    setStep('payment');
  };

  const handlePaymentConfirm = () => {
    setStep('payment-confirm');
    // Имитация проверки оплаты (3 секунды)
    setTimeout(() => {
      setStep('shuffling');
    }, 3000);
  };

  const handleCutDeck = async () => {
    setStep('cutting');
    setLoading(true);
    setError('');

    try {
      // ИСПРАВЛЕНО: используем импортированный cardNameToNumber вместо require()
      const allCards = Object.keys(cardNameToNumber);
      const shuffled = [...allCards].sort(() => Math.random() - 0.5);
      const selectedCards: CardData[] = shuffled.slice(0, selectedSpread!.cardCount).map(name => ({
        name,
        is_reversed: Math.random() < 0.3,
      }));

      console.log('Sending request to backend:', {
        question,
        spread: selectedSpread!.name,
        cards: selectedCards,
      });

      const response = await fetch('/api/read-spread', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          question,
          spread: selectedSpread!.name,
          cards: selectedCards,
        }),
      });

      console.log('Backend response status:', response.status);

      if (!response.ok) {
        const errorText = await response.text();
        console.error('Backend error:', errorText);
        throw new Error(`Ошибка сервера: ${response.status}`);
      }

      const data: ReadingResult = await response.json();
      console.log('Backend response:', data);
      
      setResult(data);
      setStep('revealing');
      
      // Поэтапное отображение карт
      await revealCardsSequentially(data.cards);
    } catch (err) {
      console.error('Error in handleCutDeck:', err);
      const errorMessage = err instanceof Error ? err.message : 'Произошла неизвестная ошибка';
      setError(`Не удалось получить интерпретацию: ${errorMessage}. Попробуйте ещё раз.`);
      setStep('shuffling'); // Возвращаемся на шаг тасовки, а не выбора расклада
    } finally {
      setLoading(false);
    }
  };

  const revealCardsSequentially = async (cards: InterpretationData[]) => {
    for (let i = 0; i < cards.length; i++) {
      await new Promise(resolve => setTimeout(resolve, 2000));
      setRevealedCards(prev => [...prev, cards[i]]);
      setCurrentCardIndex(i + 1);
    }
    setStep('result');
  };

  const handleNewReading = () => {
    setStep('welcome');
    setQuestion('');
    setSelectedSpread(null);
    setPaymentToken('');
    setRevealedCards([]);
    setCurrentCardIndex(0);
    setResult(null);
    setError('');
  };

  return (
    <div className="min-h-screen py-8 px-4">
      <div className="max-w-4xl mx-auto">
        {/* Заголовок */}
        <header className="text-center mb-12">
          <h1 className="text-5xl md:text-6xl font-bold mb-4 animate-float">
            ✨ Arcana AI ✨
          </h1>
          <p className="text-xl text-mystic-silver font-light">
            Мудрость Таро с искусственным интеллектом
          </p>
        </header>

        {/* Основной контент */}
        <div className="glass p-8 md:p-12">
          {step === 'welcome' && <WelcomeStep onStart={handleStart} />}
          {step === 'question' && (
            <QuestionStep
              question={question}
              setQuestion={setQuestion}
              onSubmit={handleQuestionSubmit}
              error={error}
            />
          )}
          {step === 'spread-select' && (
            <SpreadSelectStep
              spreads={spreads}
              onSelect={handleSpreadSelect}
              error={error}
            />
          )}
          {step === 'payment' && selectedSpread && (
            <PaymentStep
              spread={selectedSpread}
              token={paymentToken}
              onConfirm={handlePaymentConfirm}
            />
          )}
          {step === 'payment-confirm' && <PaymentConfirmStep />}
          {step === 'shuffling' && (
            <ShufflingStep 
              onCut={handleCutDeck} 
              loading={loading}
              error={error}
            />
          )}
          {step === 'cutting' && <CuttingStep />}
          {step === 'revealing' && result && (
            <RevealingStep
              result={result}
              revealedCards={revealedCards}
              currentCardIndex={currentCardIndex}
            />
          )}
          {step === 'result' && result && (
            <ResultStep result={result} onNewReading={handleNewReading} />
          )}
        </div>

        {/* Футер */}
        <footer className="text-center mt-8 text-mystic-silver text-sm">
          <p>© 2026 Arcana AI. Все расклады анонимны.</p>
        </footer>
      </div>
    </div>
  );
}

// ===== КОМПОНЕНТЫ ЭТАПОВ =====

function WelcomeStep({ onStart }: { onStart: () => void }) {
  return (
    <div className="text-center">
      <div className="text-6xl mb-6">🔮</div>
      <h2 className="text-3xl mb-6">Добро пожаловать в мир Таро</h2>
      <p className="text-lg text-mystic-silver mb-8 leading-relaxed">
        Получите глубокий и персональный расклад Таро с интерпретацией от искусственного интеллекта.
        Каждый расклад уникален и основан на мудрости веков, усиленной современными технологиями.
      </p>
      <button onClick={onStart} className="btn-gold text-lg">
        Начать расклад
      </button>
    </div>
  );
}

function QuestionStep({
  question,
  setQuestion,
  onSubmit,
  error,
}: {
  question: string;
  setQuestion: (q: string) => void;
  onSubmit: () => void;
  error: string;
}) {
  return (
    <div>
      <h2 className="text-3xl mb-6 text-center">Задайте ваш вопрос</h2>
      <p className="text-mystic-silver mb-6 text-center">
        Сформулируйте вопрос четко и конкретно. Избегайте вопросов с ответом "да/нет".
      </p>
      <textarea
        value={question}
        onChange={(e) => setQuestion(e.target.value)}
        placeholder="Например: Что меня ждет в новом проекте?"
        className="input-mystic min-h-[120px] resize-none mb-4"
        autoFocus
      />
      {error && <p className="text-red-400 text-sm mb-4">{error}</p>}
      <button onClick={onSubmit} className="btn-gold w-full">
        Продолжить
      </button>
    </div>
  );
}

function SpreadSelectStep({
  spreads,
  onSelect,
  error,
}: {
  spreads: Spread[];
  onSelect: (s: Spread) => void;
  error: string;
}) {
  return (
    <div>
      <h2 className="text-3xl mb-6 text-center">Выберите тип расклада</h2>
      {error && (
        <div className="glass p-4 mb-6 border-red-500">
          <p className="text-red-400">{error}</p>
        </div>
      )}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 max-h-[600px] overflow-y-auto pr-2">
        {spreads.map((spread) => (
          <button
            key={spread.id}
            onClick={() => onSelect(spread)}
            className="glass p-6 text-left hover:border-mystic-gold transition-all hover:scale-105"
          >
            <h3 className="text-xl mb-2">{spread.name}</h3>
            <p className="text-mystic-silver text-sm mb-3">{spread.description}</p>
            <div className="flex justify-between items-center">
              <span className="text-xs text-mystic-silver">
                {spread.cardCount} {spread.cardCount === 1 ? 'карта' : 'карт'}
              </span>
              <span className="text-mystic-gold font-bold">{spread.price}₽</span>
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}

function PaymentStep({
  spread,
  token,
  onConfirm,
}: {
  spread: Spread;
  token: string;
  onConfirm: () => void;
}) {
  return (
    <div className="text-center">
      <h2 className="text-3xl mb-6">Оплата расклада</h2>
      <div className="glass p-6 mb-6 text-left">
        <p className="text-lg mb-2">
          <strong>Расклад:</strong> {spread.name}
        </p>
        <p className="text-2xl text-mystic-gold font-bold mb-4">
          Стоимость: {spread.price}₽
        </p>
        <p className="text-sm text-mystic-silver mb-2">
          Номер платежа: <code className="text-xs">{token}</code>
        </p>
        <p className="text-sm text-mystic-silver">
          Токен действителен 1 час
        </p>
      </div>
      <div className="glass p-6 mb-6">
        <p className="text-lg mb-4">Переведите сумму на карту:</p>
        <p className="text-2xl font-mono text-mystic-gold mb-4">
          2200 0000 0000 0000
        </p>
        <p className="text-sm text-mystic-silver">
          После перевода нажмите кнопку ниже
        </p>
      </div>
      <button onClick={onConfirm} className="btn-gold text-lg">
        ✅ Я оплатил
      </button>
    </div>
  );
}

function PaymentConfirmStep() {
  return (
    <div className="text-center">
      <div className="text-6xl mb-6 animate-pulse">⏳</div>
      <h2 className="text-3xl mb-4">Проверяем оплату...</h2>
      <p className="text-mystic-silver">Это займет несколько секунд</p>
    </div>
  );
}

function ShufflingStep({ 
  onCut, 
  loading,
  error,
}: { 
  onCut: () => void;
  loading: boolean;
  error: string;
}) {
  return (
    <div className="text-center">
      <div className="text-6xl mb-6 animate-float">🌪️</div>
      <h2 className="text-3xl mb-4">Перемешиваем колоду...</h2>
      <p className="text-mystic-silver mb-8">
        Карты заряжаются вашим намерением
      </p>
      {error && (
        <div className="glass p-4 mb-6 border-red-500">
          <p className="text-red-400">{error}</p>
        </div>
      )}
      <button 
        onClick={onCut} 
        className="btn-gold text-lg"
        disabled={loading}
      >
        {loading ? '⏳ Загрузка...' : '✂️ Срезать колоду'}
      </button>
    </div>
  );
}

function CuttingStep() {
  return (
    <div className="text-center">
      <div className="text-6xl mb-6 animate-pulse">🃏</div>
      <h2 className="text-3xl mb-4">Выкладываем карты...</h2>
      <p className="text-mystic-silver">Запрашиваем мудрость ИИ...</p>
    </div>
  );
}

function RevealingStep({
  result,
  revealedCards,
  currentCardIndex,
}: {
  result: ReadingResult;
  revealedCards: InterpretationData[];
  currentCardIndex: number;
}) {
  return (
    <div>
      {result.intro && (
        <div className="mb-8 p-6 glass">
          <p className="text-lg leading-relaxed">{result.intro}</p>
        </div>
      )}
      
      <div className="space-y-6">
        {revealedCards.map((card, index) => (
          <div key={index} className="glass p-6">
            <div className="flex flex-col md:flex-row gap-6">
              <div className="flex-shrink-0">
                <img
                  src={getCardImageUrl(card.card.name)}
                  alt={card.card.name}
                  className="w-48 h-auto rounded-lg shadow-2xl"
                />
              </div>
              <div className="flex-1">
                <h3 className="text-2xl mb-2">
                  Позиция {index + 1}: {card.card.name}
                  {card.card.is_reversed && ' 🔄'}
                </h3>
                <p className="text-mystic-silver leading-relaxed">
                  {card.interpretation}
                </p>
              </div>
            </div>
          </div>
        ))}
      </div>

      {currentCardIndex < result.cards.length && (
        <div className="text-center mt-8">
          <div className="text-4xl animate-pulse">✨</div>
          <p className="text-mystic-silver mt-2">Раскрываем следующую карту...</p>
        </div>
      )}
    </div>
  );
}

function ResultStep({
  result,
  onNewReading,
}: {
  result: ReadingResult;
  onNewReading: () => void;
}) {
  return (
    <div className="text-center">
      {result.conclusion && (
        <div className="glass p-6 mb-8">
          <h3 className="text-2xl mb-4">Итог</h3>
          <p className="text-lg leading-relaxed">{result.conclusion}</p>
        </div>
      )}
      
      <div className="glass p-6 mb-8">
        <p className="text-xl text-mystic-gold">
          ✨ Если у вас есть ещё вопросы, начните новый расклад!
        </p>
      </div>

      <button onClick={onNewReading} className="btn-gold text-lg">
        🔮 Новый расклад
      </button>
    </div>
  );
}
