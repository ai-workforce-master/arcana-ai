'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { motion, AnimatePresence } from 'framer-motion';
import { spreads, categoryNames, Spread } from '@/lib/spreads';

type Step = 'welcome' | 'question' | 'spread-select' | 'payment';

const fadeIn = {
  initial: { opacity: 0, y: 20 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -20 },
  transition: { duration: 0.5 },
};

const BACKEND_URL = process.env.NEXT_PUBLIC_BACKEND_URL || 'https://ai-arcana.ru';

export default function HomePage() {
  const router = useRouter();
  const [step, setStep] = useState<Step>('welcome');
  const [question, setQuestion] = useState('');
  const [selectedSpread, setSelectedSpread] = useState<Spread | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const handleStart = () => setStep('question');

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
    setStep('payment');
  };

  const handlePaymentConfirm = async () => {
    setLoading(true);
    setError('');
    
    try {
      const sessionId = crypto.randomUUID();
      localStorage.setItem('arcana_session_id', sessionId);
      localStorage.setItem('arcana_question', question);
      localStorage.setItem('arcana_spread', JSON.stringify(selectedSpread));

      const response = await fetch(`${BACKEND_URL}/api/create-payment`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          session_id: sessionId,
          spread_name: selectedSpread!.name,
          amount: selectedSpread!.price,
        }),
      });

      if (!response.ok) {
        const errData = await response.json().catch(() => ({}));
        throw new Error(errData.error || `HTTP Error: ${response.status}`);
      }

      const data = await response.json();
      console.log('Payment created:', data);
      
      // ВАЖНО: Сохраняем ID платежа, чтобы страница успеха знала, что проверять
      localStorage.setItem('arcana_payment_id', data.payment_id);
      
      window.location.href = data.confirmation_url;
      
    } catch (err: any) {
      console.error('Payment error:', err);
      setError(`Ошибка: ${err.message || 'Неизвестная ошибка'}`);
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen py-8 px-4">
      <div className="max-w-5xl mx-auto">
        <motion.header className="text-center mb-8" initial={{ opacity: 0, y: -30 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.8 }}>
          <h1 className="text-4xl md:text-5xl font-bold mb-2 text-yellow-400">✨ Arcana AI ✨</h1>
          <p className="text-lg text-gray-300 font-light">Мудрость Таро с искусственным интеллектом</p>
        </motion.header>

        <AnimatePresence mode="wait">
          <motion.div key={step} className="glass p-8 md:p-12" initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }} exit={{ opacity: 0, scale: 0.95 }} transition={{ duration: 0.4 }}>
            {step === 'welcome' && (
              <motion.div className="text-center" {...fadeIn}>
                <div className="relative w-64 h-64 mx-auto mb-8" style={{ backgroundColor: "#292c4c" }}>
                  <div className="absolute inset-0 rounded-full bg-gradient-to-br from-yellow-400/20 via-purple-400/10 to-blue-400/20 blur-2xl" />
                  <img src="/arcana-logo.png" alt="Arcana AI" className="relative w-full h-full object-contain drop-shadow-2xl rounded-2xl border border-yellow-400/20 shadow-2xl" />
                </div>
                <h2 className="text-3xl md:text-4xl mb-6 text-yellow-400">Добро пожаловать в мир Таро</h2>
                <p className="text-lg text-gray-300 mb-8 leading-relaxed max-w-2xl mx-auto">Получите глубокий и персональный расклад Таро с интерпретацией от искусственного интеллекта.</p>
                <button onClick={handleStart} className="btn-gold text-lg pulse-gold">Начать расклад</button>
              </motion.div>
            )}
            
            {step === 'question' && (
              <motion.div {...fadeIn}>
                <h2 className="text-3xl mb-6 text-center text-yellow-400">Задайте ваш вопрос</h2>
                <textarea value={question} onChange={(e) => setQuestion(e.target.value)} placeholder="Например: Что меня ждет в новом проекте?" className="input-mystic min-h-[120px] resize-none mb-4 w-full p-3 rounded bg-gray-800/50 border border-yellow-400/30 text-white" autoFocus />
                {error && <p className="text-red-400 text-sm mb-4">{error}</p>}
                <button onClick={handleQuestionSubmit} className="btn-gold w-full">Продолжить</button>
              </motion.div>
            )}

            {step === 'spread-select' && (
              <motion.div {...fadeIn}>
                <h2 className="text-3xl mb-6 text-center text-yellow-400">Выберите тип расклада</h2>
                <div className="max-h-[600px] overflow-y-auto pr-2 space-y-6">
                  {(['simple', 'medium', 'complex'] as const).map(category => (
                    <div key={category}>
                      <h3 className="text-xl mb-3 text-yellow-400">{categoryNames[category]}</h3>
                      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                        {spreads.filter(s => s.category === category).map(spread => (
                          <motion.button key={spread.id} onClick={() => handleSpreadSelect(spread)} className="glass p-5 text-left hover:border-yellow-400 transition-all" whileHover={{ scale: 1.03 }} whileTap={{ scale: 0.98 }}>
                            <h4 className="text-lg mb-1 text-yellow-300">{spread.name}</h4>
                            <p className="text-gray-400 text-sm mb-3">{spread.description}</p>
                            <div className="flex justify-between items-center">
                              <span className="text-xs text-gray-500">{spread.cardCount} {spread.cardCount === 1 ? 'карта' : 'карт'}</span>
                              <span className="text-yellow-400 font-bold text-lg">{spread.price}₽</span>
                            </div>
                          </motion.button>
                        ))}
                      </div>
                    </div>
                  ))}
                </div>
              </motion.div>
            )}

            {step === 'payment' && selectedSpread && (
              <motion.div className="text-center" {...fadeIn}>
                <h2 className="text-3xl mb-6 text-yellow-400">Оплата расклада</h2>
                <div className="glass p-6 mb-6 text-left">
                  <p className="text-lg mb-2"><strong>Расклад:</strong> {selectedSpread.name}</p>
                  <p className="text-3xl text-yellow-400 font-bold mb-4">Стоимость: {selectedSpread.price}₽</p>
                  <p className="text-sm text-gray-400">Безопасная оплата через ЮKassa</p>
                </div>
                {error && <div className="glass p-4 mb-6 border border-red-500/50 bg-red-500/10"><p className="text-red-400 text-sm">{error}</p></div>}
                <button onClick={handlePaymentConfirm} className="btn-gold text-lg w-full" disabled={loading}>
                  {loading ? '⏳ Создание ссылки на оплату...' : '💳 Перейти к оплате'}
                </button>
              </motion.div>
            )}
          </motion.div>
        </AnimatePresence>
      </div>
    </div>
  );
}
